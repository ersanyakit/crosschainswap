package venue

type UniswapV2Config struct {
	FactoryAddress string
	RouterAddress  string

	InitCodeHash    string
	DeploymentBlock uint64
}

func (UniswapV2Config) VenueConfigKind() VenueKind {
	return VenueKindUniswapV2
}

type UniswapV3Config struct {
	FactoryAddress         string
	RouterAddress          string
	QuoterAddress          string
	PositionManagerAddress string

	DeploymentBlock uint64
}

func (UniswapV3Config) VenueConfigKind() VenueKind {
	return VenueKindUniswapV3
}

type UniswapV4Config struct {
	PoolManagerAddress string
	UniversalRouter    string
	QuoterAddress      string

	DeploymentBlock uint64
}

func (UniswapV4Config) VenueConfigKind() VenueKind {
	return VenueKindUniswapV4
}

type RaydiumConfig struct {
	AMMProgramID      string
	CLMMProgramID     string
	CPMMProgramID     string
	OpenBookProgramID string
}

func (RaydiumConfig) VenueConfigKind() VenueKind {
	return VenueKindRaydium
}

type OrcaConfig struct {
	WhirlpoolProgramID string
	ConfigAccounts     []string
}

func (OrcaConfig) VenueConfigKind() VenueKind {
	return VenueKindOrca
}

type MeteoraConfig struct {
	DLMMProgramID string
}

func (MeteoraConfig) VenueConfigKind() VenueKind {
	return VenueKindMeteora
}

type AerodromeClassicConfig struct {
	PoolFactoryAddress string
	RouterAddress      string
	VoterAddress       string
	DeploymentBlock    uint64
}

func (AerodromeClassicConfig) VenueConfigKind() VenueKind {

	return VenueKindAerodrome

}
