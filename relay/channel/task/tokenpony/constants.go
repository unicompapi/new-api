package tokenpony

const (
	ChannelName        = "tokenpony-seedance"
	GenerateEndpoint   = "/v1/aigc/video/generate"
	TaskEndpoint       = "/v1/aigc/tasks/"
	AssetsEndpoint     = "/v1/assets/"
	ModelSeedance20    = "doubao-seedance-2-0-260128"
	ModelSeedance25    = "doubao-seedance-2-5-260628"
	defaultHTTPTimeout = 30
)

var ModelList = []string{ModelSeedance20, ModelSeedance25}

const (
	seedance20BaseTokenPrice = 46.0
	seedance25BaseTokenPrice = 70.0
)

var allowedAssetActions = map[string]struct{}{
	"CreateAssetGroup":            {},
	"GetAssetGroup":               {},
	"UpdateAssetGroup":            {},
	"CreateVisualValidateSession": {},
	"GetVisualValidateResult":     {},
	"CreateAsset":                 {},
	"ListAssets":                  {},
	"GetAsset":                    {},
	"UpdateAsset":                 {},
	"DeleteAsset":                 {},
}

func IsAssetActionAllowed(action string) bool {
	_, ok := allowedAssetActions[action]
	return ok
}

func IsLivenessAction(action string) bool {
	return action == "CreateVisualValidateSession" || action == "GetVisualValidateResult"
}
