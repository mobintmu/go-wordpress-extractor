// Command server runs the stateless product extraction service.
package main

import (
	"go-wordpress/internal/app"
	"go-wordpress/internal/config"
)

//	@title			Product Extraction API
//	@version		1.0
//	@description	Extracts one product from one product page using per-site configuration.
//	@description	Each request responds within 3 minutes. Both extraction results return HTTP 200, so clients must read the status field.
//	@BasePath		/

//	@tag.name			Extraction
//	@tag.description	Product extraction for client applications.
//	@tag.name			Operations
//	@tag.description	Health, readiness, and operator endpoints.

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key
//	@description				Client API key: gwp_<id>_<secret>

//	@securityDefinitions.basic	OpsBasicAuth
//	@description				Operator credentials (APP_OPS_USER and APP_OPS_PASSWORD_HASH) for Swagger UI and /metrics.

func main() {
	config.LoadEnv()
	app.NewApp().Run()
}
