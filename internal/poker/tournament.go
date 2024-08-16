package poker

type Tournament struct {
	ID                    string
	Name                  string
	RegistrationStartDate string
	RegistrationEndDate   string
	StartDate             string
	EndDate               string
	Prize                 string
	Configuration         string
	Ongoing               bool
	MinPlayers            int
	MaxPlayers            int
	TurnSeconds           int
}
