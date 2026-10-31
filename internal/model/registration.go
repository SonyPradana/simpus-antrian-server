package model

type RegistrationOption struct {
	ID   string `json:"id"`
	Name string `json:"nama"`
}

type RegistrationSelection struct {
	PetugasID        string `json:"petugas_id"`
	JenisPasienID    string `json:"jenis_pasien_id"`
	JenisPuskesmasID string `json:"jenis_puskesmas_id"`
	JenisTindakanID  string `json:"jenis_tindakan_id"`
	JenisPengirimID  string `json:"jenis_pengirim_id"`
	JenisKunjunganID string `json:"jenis_kunjungan_id"`
}

type RegistrationOptions struct {
	Petugas        []RegistrationOption  `json:"petugas"`
	JenisPasien    []RegistrationOption  `json:"jenis_pasien"`
	JenisPuskesmas []RegistrationOption  `json:"jenis_puskesmas"`
	JenisTindakan  []RegistrationOption  `json:"jenis_tindakan"`
	JenisPengirim  []RegistrationOption  `json:"jenis_pengirim"`
	JenisKunjungan []RegistrationOption  `json:"jenis_kunjungan"`
	Selected       RegistrationSelection `json:"terpilih"`
	Location       string                `json:"lokasi,omitempty"`
	ConfigFile     string                `json:"config_file,omitempty"`
}

type QueueOnlyResult struct {
	Type       string `json:"jenis"`
	Poli       Poli   `json:"poli"`
	Doctor     Doctor `json:"dokter"`
	Queue      Queue  `json:"antrian"`
	PoscheckID string `json:"poscheck_id,omitempty"`
	Location   string `json:"lokasi,omitempty"`
	DryRun     bool   `json:"dry_run"`
}
