package service

import (
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishedPriceProvenanceUsesFrozenRequestSnapshot(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	expression := `tier("short", p * 2 + c * 10 + cr * 0.1 + cc * 2.5)`
	snapshot := &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ModelName: "published-fixture", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1,
		OfficialPricePublicationID: strings.Repeat("a", 64), OfficialPriceSourceSHA256: strings.Repeat("b", 64)}
	info := &relaycommon.RelayInfo{OriginModelName: "published-fixture", UsingGroup: "default", UserGroup: "default", TieredBillingSnapshot: snapshot}
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"published-fixture":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"published-fixture":"p * 999"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, info, nil)
	ref, ok := other["official_price_source"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, snapshot.OfficialPricePublicationID, ref["publication_id"])
	assert.Equal(t, snapshot.OfficialPriceSourceSHA256, ref["source_sha256"])
	assert.Equal(t, snapshot.ExprHash, ref["expression_sha256"])
	assert.Equal(t, "reference_tariff_not_upstream_invoice", ref["scope"])
	assert.NotContains(t, other, "actual_api_cost_usd", "a tariff reference cannot invent an upstream charge")
	manual := &relaycommon.RelayInfo{TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: "p * 3"}}
	manualLog := map[string]interface{}{}
	InjectTieredBillingInfo(manualLog, manual, nil)
	assert.NotContains(t, manualLog, "official_price_source")
}
