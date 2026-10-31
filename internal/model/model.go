package model

import "time"

type Patient struct {
	ID            string    `json:"no_rm"`
	Nama          string    `json:"nama"`
	NamaKK        string    `json:"nama_kk,omitempty"`
	Alamat        string    `json:"alamat"`
	Dusun         string    `json:"dusun,omitempty"`
	Desa          string    `json:"desa,omitempty"`
	TglLahir      string    `json:"tanggal_lahir,omitempty"`
	NoKartu       string    `json:"no_bpjs,omitempty"`
	NIK           string    `json:"nik,omitempty"`
	SexID         string    `json:"sex_id,omitempty"`
	WorkID        string    `json:"work_id,omitempty"`
	OrchardID     string    `json:"orchard_id,omitempty"`
	VillageID     string    `json:"village_id,omitempty"`
	TypepatientID string    `json:"typepatient_id,omitempty"`
	EducationID   string    `json:"education_id,omitempty"`
	JenisPasien   string    `json:"jenis_pasien,omitempty"`
	IsBPJS        bool      `json:"is_bpjs"`
	Gakin         bool      `json:"gakin"`
	Bayar         bool      `json:"bayar"`
	Prolanis      string    `json:"prolanis,omitempty"`
	PRB           string    `json:"prb,omitempty"`
	TglLahirTime  time.Time `json:"-"`
}

type BPJSStatus struct {
	IsBPJS          bool   `json:"is_bpjs"`
	NoKartu         string `json:"no_kartu,omitempty"`
	Source          string `json:"sumber"`
	Verified        bool   `json:"terverifikasi"`
	Active          *bool  `json:"aktif,omitempty"`
	Status          string `json:"status,omitempty"`
	JenisPeserta    string `json:"jenis_peserta,omitempty"`
	Kelas           string `json:"kelas,omitempty"`
	ProviderCode    string `json:"kode_provider,omitempty"`
	ProviderName    string `json:"nama_provider,omitempty"`
	TglMulaiAktif   string `json:"tgl_mulai_aktif,omitempty"`
	TglAkhirBerlaku string `json:"tgl_akhir_berlaku,omitempty"`
	Message         string `json:"pesan,omitempty"`
}

type Poli struct {
	ID            string `json:"id"`
	Code          string `json:"kode_antrian"`
	UnitID        string `json:"unit_id"`
	Name          string `json:"nama"`
	BPJSCode      string `json:"kode_bpjs"`
	PoliSakit     bool   `json:"poli_sakit"`
	Prefix        string `json:"prefix"`
	PoliHFIS      string `json:"kode_hfis,omitempty"`
	UnitName      string `json:"nama_unit,omitempty"`
	QueueUnitName string `json:"nama_unit_antrian,omitempty"`
	QueueTotal    int    `json:"total_antrian,omitempty"`
}

type Doctor struct {
	Code     string `json:"kode_dokter"`
	Name     string `json:"nama_dokter"`
	Schedule string `json:"jam_praktek"`
	Capacity int    `json:"kapasitas"`
	PoliCode string `json:"kode_poli"`
}

type Queue struct {
	Code           string `json:"kode"`
	Number         int    `json:"no_urut_poli"`
	GlobalNumber   int    `json:"no_urut"`
	Date           string `json:"tanggal"`
	Waktu          string `json:"waktu,omitempty"`
	Poli           string `json:"poli"`
	DoctorName     string `json:"dokter"`
	DoctorCode     string `json:"kode_dokter"`
	WaitingAhead   int    `json:"menunggu_di_depan"`
	RemainingQuota int    `json:"sisa_kuota"`
}

type RegistrationResult struct {
	Patient    Patient    `json:"patient"`
	BPJS       BPJSStatus `json:"bpjs"`
	Poli       Poli       `json:"poli"`
	Doctor     Doctor     `json:"dokter"`
	Queue      Queue      `json:"antrian"`
	PetugasID  string     `json:"petugas_id,omitempty"`
	PoscheckID string     `json:"poscheck_id,omitempty"`
	Location   string     `json:"lokasi,omitempty"`
	VisitID    int64      `json:"visit_id,omitempty"`
	DryRun     bool       `json:"dry_run"`
	Existing   bool       `json:"sudah_terdaftar,omitempty"`
}
