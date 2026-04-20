package core

import "github.com/gin-gonic/gin"

// WrapAuthMiddleware layers wrap on top of the coreapp auth middleware
// installed by NewServerConfig. Inside wrap, call next(c) to delegate to the
// underlying auth chain (API token + provider-based auth); skip the call to
// short-circuit it.
//
// The wrap takes effect immediately for every registered route (core + module
// handlers) because APIOptions.Middlewares stores a stable method value bound
// to an internal slot — no slice position, no function-pointer identification,
// no reflection.
//
// Safe to call multiple times: wrappers compose in call order, so the last
// WrapAuthMiddleware call runs outermost.
//
// Panics on clear misuse: nil receiver, nil wrap, or a ServerConfig that
// wasn't produced by NewServerConfig. These are programmer errors — failing
// fast at startup is preferable to shipping a silently-broken auth chain.
func (sc *ServerConfig) WrapAuthMiddleware(wrap func(next gin.HandlerFunc) gin.HandlerFunc) {
	if sc == nil {
		panic("WrapAuthMiddleware: ServerConfig is nil")
	}
	if wrap == nil {
		panic("WrapAuthMiddleware: wrap function is nil")
	}
	if sc.authSlot == nil {
		panic("WrapAuthMiddleware: ServerConfig.authSlot is nil — ServerConfig was not built via NewServerConfig")
	}
	sc.authSlot.inner = wrap(sc.authSlot.inner)
}
