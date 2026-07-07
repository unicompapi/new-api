package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestModelPriceToUSDHappyHorse(t *testing.T) {
	operation_setting.USDExchangeRate = 7.3
	require.True(t, IsCNYModelPrice("happyhorse-1.1-t2v"))
	require.InDelta(t, 0.9/7.3, ModelPriceToUSD("happyhorse-1.1-t2v", 0.9), 0.000001)
}

func TestModelPriceToUSDRegularModel(t *testing.T) {
	operation_setting.USDExchangeRate = 7.3
	require.False(t, IsCNYModelPrice("sora-2"))
	require.Equal(t, 0.3, ModelPriceToUSD("sora-2", 0.3))
}

func TestModelPriceToUSDFallsBackWhenRateInvalid(t *testing.T) {
	operation_setting.USDExchangeRate = 0
	require.InDelta(t, div(0.9, USD2RMB), ModelPriceToUSD("happyhorse-1.0-t2v", 0.9), 0.000001)
}

func div(a, b float64) float64 {
	return a / b
}
