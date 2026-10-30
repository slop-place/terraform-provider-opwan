package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*metaDataSource)(nil)

type metaDataSource struct {
	client *client.Client
}

type metaModel struct {
	Version      string `tfsdk:"version"`
	APIVersion   string `tfsdk:"api_version"`
	TermsVersion string `tfsdk:"terms_version"`
	TermsURL     string `tfsdk:"terms_url"`
}

// NewMetaDataSource returns the opwan_meta data source.
func NewMetaDataSource() datasource.DataSource {
	return &metaDataSource{}
}

func (d *metaDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_meta"
}

func (d *metaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The opwan server's version and terms of service. Needs no API token.",
		Attributes: map[string]schema.Attribute{
			"version":       computedString("The server version."),
			"api_version":   computedString("The API version."),
			"terms_version": computedString("The terms of service version in effect."),
			"terms_url":     computedString("Where the terms of service are published."),
		},
	}
}

func (d *metaDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *metaDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	meta, err := d.client.GetMeta(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read opwan's version", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, metaModel{
		Version:      meta.Version,
		APIVersion:   meta.ApiVersion,
		TermsVersion: meta.TermsVersion,
		TermsURL:     meta.TermsUrl,
	})...)
}
