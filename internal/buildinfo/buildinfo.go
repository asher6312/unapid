package buildinfo

const (
	Name           = "UnAPI'd"
	Version        = "2.1.0"
	RuntimeRoot    = "/var/lib/unapid/runtime"
	OwnerFile      = RuntimeRoot + "/owner.json"
	StateRoot      = "/var/lib/unapid/state"
	StateOwnerFile = StateRoot + "/owner.json"
	Project        = "unapid"
	APIService     = "api"
	OAuthService   = "translator"
	GatewayHost    = "subscription-api-gateway"
	GatewayPort    = 8317
	OAuthPort      = 8318
	ManagedNetwork = "unapid_private"
	NetworkLabel   = "io.unapid.network.managed"
	OwnerLabel     = "io.unapid.runtime.managed"
)
