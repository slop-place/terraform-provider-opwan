package provider

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

type tunnelStatusModel struct {
	Up            bool    `tfsdk:"up"`
	LastHandshake *string `tfsdk:"last_handshake"`
	RateBpsRx     int64   `tfsdk:"rate_bps_rx"`
	RateBpsTx     int64   `tfsdk:"rate_bps_tx"`
	Throttled     string  `tfsdk:"throttled"`
	UpdatedAt     *string `tfsdk:"updated_at"`
}

// THROTTLED_DESCRIPTION documents every attribute carrying a throttled reason.
const THROTTLED_DESCRIPTION = "Why the speed is lowered right now: `none`, `burst`, `allowance` or `pacing`."

func tunnelStatusAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"up":             computedBool("A handshake within the last 3 minutes."),
		"last_handshake": computedString("The last WireGuard handshake (RFC 3339), null before the first."),
		"rate_bps_rx":    computedInt64("Bits per second to you, over the last report."),
		"rate_bps_tx":    computedInt64("Bits per second from you, over the last report."),
		"throttled":      schema.StringAttribute{Computed: true, MarkdownDescription: THROTTLED_DESCRIPTION},
		"updated_at":     computedString("When the exit last reported (RFC 3339), null before the first report."),
	}
}

func tunnelStatusFromAPI(status *opwanapi.TunnelStatus) *tunnelStatusModel {
	if status == nil {
		return nil
	}
	return &tunnelStatusModel{
		Up:            status.Up,
		LastHandshake: formatTime(status.LastHandshake),
		RateBpsRx:     status.RateBpsRx,
		RateBpsTx:     status.RateBpsTx,
		Throttled:     string(status.Throttled),
		UpdatedAt:     formatTime(status.UpdatedAt),
	}
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
}

func computedString(description string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, MarkdownDescription: description}
}

func computedInt64(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Computed: true, MarkdownDescription: description}
}

func computedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Computed: true, MarkdownDescription: description}
}

func computedStringList(description string) schema.ListAttribute {
	return schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: description}
}
