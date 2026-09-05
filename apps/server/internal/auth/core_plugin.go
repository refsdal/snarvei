package auth

import (
	"errors"

	"github.com/thecodearcher/limen"
)

// corePlugin exists because *limen.LimenCore is not reachable from the
// *limen.Limen handle, but every plugin receives it in Initialize. It keeps
// the pointer so this package can refresh cookies, create sessions for the
// invitation-register flow and read users through DBAction.
type corePlugin struct{ core *limen.LimenCore }

func (p *corePlugin) Name() limen.PluginName { return "snarvei-core" }

func (p *corePlugin) Initialize(core *limen.LimenCore) error {
	if core == nil {
		return errors.New("auth: limen initialized the plugin with a nil core")
	}
	p.core = core
	return nil
}

func (p *corePlugin) PluginHTTPConfig() limen.PluginHTTPConfig { return limen.PluginHTTPConfig{} }

func (p *corePlugin) RegisterRoutes(*limen.LimenHTTPCore, *limen.RouteBuilder) {}

var _ limen.Plugin = (*corePlugin)(nil)
