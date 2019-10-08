package common

type Rtt struct {
	Avg float64 `desc:"平均延遲"`
	Min float64 `desc:"最小延遲"`
	Max float64 `desc:"最大延遲"`
	Jitter float64 `desc:"抖動延遲"`
	Loss float64 `desc:"丟包率"`
}

type NetWorkStatus struct {
	Rtt
	BandWidth float64 `desc:"網絡帶寬，以Mbit/s為單位"`
}