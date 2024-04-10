package pro1pm

type Switch struct {
	Id      int     `json:"id"`
	Source  string  `json:"source"`
	Output  bool    `json:"output"`
	Apower  float64 `json:"apower"`
	Voltage float64 `json:"voltage"`
	Freq    float64 `json:"freq"`
	Current float64 `json:"current"`
	Pf      float64 `json:"pf"`
	Aenergy struct {
		Total    float64   `json:"total"`
		ByMinute []float64 `json:"by_minute"`
		MinuteTs int       `json:"minute_ts"`
	} `json:"aenergy"`
	RetAenergy struct {
		Total    float64   `json:"total"`
		ByMinute []float64 `json:"by_minute"`
		MinuteTs int       `json:"minute_ts"`
	} `json:"ret_aenergy"`
	Temperature struct {
		TC float64 `json:"tC"`
		TF float64 `json:"tF"`
	} `json:"temperature"`
}
