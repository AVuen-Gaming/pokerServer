package codec

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"
)

// GzipDataConverter es un DataConverter que comprime el payload si supera un umbral.
type GzipDataConverter struct {
	Threshold int // Umbral en bytes; si el payload es mayor, se comprime.
}

// NewGzipDataConverter crea un nuevo GzipDataConverter con el umbral dado.
func NewGzipDataConverter(threshold int) converter.DataConverter {
	return &GzipDataConverter{Threshold: threshold}
}

// ToPayloads convierte los valores a payloads.
func (c *GzipDataConverter) ToPayloads(values ...interface{}) (*commonpb.Payloads, error) {
	payloads := &commonpb.Payloads{}
	for _, v := range values {
		p, err := c.ToPayload(v)
		if err != nil {
			return nil, err
		}
		payloads.Payloads = append(payloads.Payloads, p)
	}
	return payloads, nil
}

// FromPayloads decodifica cada payload en valuePtrs.
func (c *GzipDataConverter) FromPayloads(payloads *commonpb.Payloads, valuePtrs ...interface{}) error {
	for i, p := range payloads.Payloads {
		if err := c.FromPayload(p, valuePtrs[i]); err != nil {
			return err
		}
	}
	return nil
}

// ToPayload convierte un valor en un payload, comprimiéndolo si es grande.
func (c *GzipDataConverter) ToPayload(value interface{}) (*commonpb.Payload, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	metadata := map[string][]byte{
		converter.MetadataEncoding: []byte("json/plain"),
	}
	// Si el payload es mayor que el umbral, lo comprime con gzip.
	if len(data) > c.Threshold {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(data)
		if err != nil {
			return nil, err
		}
		err = gw.Close()
		if err != nil {
			return nil, err
		}
		data = buf.Bytes()
		metadata[converter.MetadataEncoding] = []byte("gzip+json")
	}
	return &commonpb.Payload{
		Data:     data,
		Metadata: metadata,
	}, nil
}

// FromPayload decodifica un payload, descomprimiéndolo si es necesario.
func (c *GzipDataConverter) FromPayload(payload *commonpb.Payload, valuePtr interface{}) error {
	encoding := string(payload.Metadata[converter.MetadataEncoding])
	data := payload.Data
	if encoding == "gzip+json" {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		defer r.Close()
		decompressed, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		data = decompressed
	}
	return json.Unmarshal(data, valuePtr)
}

// ToString convierte un payload a string.
func (c *GzipDataConverter) ToString(payload *commonpb.Payload) string {
	return string(payload.Data)
}

// ToStrings convierte un slice de payloads a slice de strings.
func (c *GzipDataConverter) ToStrings(payloads *commonpb.Payloads) []string {
	strs := make([]string, len(payloads.Payloads))
	for i, p := range payloads.Payloads {
		strs[i] = c.ToString(p)
	}
	return strs
}

var (
	store   = make(map[string][]byte)
	mu      sync.Mutex
	counter int
)

func Save(data []byte) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	counter++
	// Generamos un ID simple basado en el timestamp y un contador.
	id := time.Now().Format("20060102150405") + "-" + // timestamp
		fmt.Sprintf("%d", counter)
	store[id] = data
	return id, nil
}

func Load(id string) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	data, ok := store[id]
	if !ok {
		return nil, errors.New("data not found")
	}
	return data, nil
}
