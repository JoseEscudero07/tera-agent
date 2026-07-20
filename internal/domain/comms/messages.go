package comms

import "encoding/json"

// ProtocolVersion is the wire protocol version this Agent speaks.
const ProtocolVersion = "1.0"

// Message type discriminators (see docs/protocol/messages.md).
const (
	// Agent -> Server
	TypeHello        = "hello"
	TypeAuthenticate = "authenticate"
	TypeRegister     = "register"
	TypeCapabilities = "capabilities"
	TypeProfilesAck  = "profiles_ack"
	TypeHeartbeat    = "heartbeat"
	TypeJobReceived  = "job_received"
	TypeJobCompleted = "job_completed"
	TypeJobFailed    = "job_failed"

	// Server -> Agent
	TypeAuthenticated = "authenticated"
	TypeAuthError     = "auth_error"
	TypeProfilesSync  = "profiles_sync"
	TypeProfileUpdate = "profile_update"
	TypeConfig        = "config"
	TypeJob           = "job"
	TypeError         = "error"
)

// Typed extracts just the discriminator from an inbound frame.
type Typed struct {
	Type string `json:"type"`
}

// --- Agent -> Server ---

type Hello struct {
	Type            string `json:"type"`
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion string `json:"protocol_version"`
	InstallationID  string `json:"installation_id"`
	MachineID       string `json:"machine_id"`
}

type Authenticate struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type Register struct {
	Type         string `json:"type"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	OSVersion    string `json:"os_version"`
	AgentVersion string `json:"agent_version"`
	LocalIP      string `json:"local_ip"`
}

type PrinterDTO struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Type   string `json:"type"`
}

type Capabilities struct {
	Type     string       `json:"type"`
	Devices  []string     `json:"devices"`
	Printers []PrinterDTO `json:"printers"`
}

type ProfilesAck struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

type Heartbeat struct {
	Type        string `json:"type"`
	State       string `json:"state"`
	UptimeS     int64  `json:"uptime_s"`
	MemBytes    uint64 `json:"mem_bytes"`
	Version     string `json:"version"`
	PendingJobs int    `json:"pending_jobs"`
}

type JobReceived struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type JobCompleted struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	DurationMS int64  `json:"duration_ms"`
}

type JobFailed struct {
	Type  string    `json:"type"`
	ID    string    `json:"id"`
	Error ErrorBody `json:"error"`
}

// --- Server -> Agent ---

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Authenticated struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	CompanyID string `json:"company_id"`
	BranchID  string `json:"branch_id"`
}

type AuthError struct {
	Type      string    `json:"type"`
	Error     ErrorBody `json:"error"`
	Retryable bool      `json:"retryable"`
}

type ProfileDTO struct {
	PrinterID      string            `json:"printer_id"`
	NativeFormats  []string          `json:"native_formats"`
	WidthDots      int               `json:"width_dots"`
	DPI            int               `json:"dpi"`
	SupportsCut    bool              `json:"supports_cut"`
	SupportsDrawer bool              `json:"supports_drawer"`
	Meta           map[string]string `json:"meta"`
}

type ProfilesSync struct {
	Type     string       `json:"type"`
	Version  int          `json:"version"`
	Profiles []ProfileDTO `json:"profiles"`
}

type ProfileUpdate struct {
	Type    string     `json:"type"`
	Version int        `json:"version"`
	Profile ProfileDTO `json:"profile"`
}

type Config struct {
	Type             string `json:"type"`
	HeartbeatSeconds int    `json:"heartbeat_seconds"`
	LogLevel         string `json:"log_level"`
}

type Job struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	JobType string          `json:"job_type"`
	Payload json.RawMessage `json:"payload"`
}
