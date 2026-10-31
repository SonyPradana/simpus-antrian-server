package model

type APMLookupResult struct {
	Mode        string      `json:"mode"`
	CanRegister bool        `json:"can_register"`
	Fallback    string      `json:"fallback,omitempty"`
	Reason      string      `json:"reason,omitempty"`
	Patient     *Patient    `json:"patient,omitempty"`
	BPJS        *BPJSStatus `json:"bpjs,omitempty"`
}

type APMOptionSelection struct {
	PetugasID  string `json:"petugas_id"`
	PoscheckID string `json:"poscheck_id"`
	Location   string `json:"lokasi"`
}

type APMOptionsResult struct {
	Petugas  []RegistrationOption `json:"petugas"`
	Poscheck []RegistrationOption `json:"poscheck"`
	Terpilih APMOptionSelection   `json:"terpilih"`
}

type APMQueueResult struct {
	Mode              string      `json:"mode"`
	NeedsRegistration bool        `json:"needs_registration"`
	Patient           *Patient    `json:"patient,omitempty"`
	BPJS              *BPJSStatus `json:"bpjs,omitempty"`
	Poli              Poli        `json:"poli"`
	Doctor            Doctor      `json:"dokter"`
	Queue             Queue       `json:"antrian"`
	PetugasID         string      `json:"petugas_id,omitempty"`
	PoscheckID        string      `json:"poscheck_id,omitempty"`
	Location          string      `json:"lokasi,omitempty"`
	VisitID           int64       `json:"visit_id,omitempty"`
	DryRun            bool        `json:"dry_run"`
	Existing          bool        `json:"sudah_terdaftar,omitempty"`
}
