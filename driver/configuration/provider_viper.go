package configuration

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rs/cors"
	"github.com/spf13/viper"

	"github.com/ory/x/corsx"

	"github.com/sirupsen/logrus"

	"github.com/ory/hydra/x"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/stringslice"
	"github.com/ory/x/tracing"
	"github.com/ory/x/urlx"
	"github.com/ory/x/viperx"
)

type ViperProvider struct {
	l                 logrus.FieldLogger
	ss                [][]byte
	generatedSecret   []byte
	forcedHTTP        bool
	insecureRedirects []string
}

const (
	ViperKeyWellKnownKeys                 = "webfinger.jwks.broadcast_keys"
	ViperKeyOAuth2ClientRegistrationURL   = "webfinger.oidc_discovery.client_registration_url"
	ViperKeyOIDCDiscoverySupportedClaims  = "webfinger.oidc_discovery.supported_claims"
	ViperKeyOIDCDiscoverySupportedScope   = "webfinger.oidc_discovery.supported_scope"
	ViperKeyOIDCDiscoveryUserinfoEndpoint = "webfinger.oidc_discovery.userinfo_url"

	ViperKeySubjectTypesSupported          = "oidc.subject_identifiers.enabled"
	ViperKeyDefaultClientScope             = "oidc.dynamic_client_registration.default_scope"
	ViperKeyDSN                            = "dsn"
	ViperKeyBCryptCost                     = "oauth2.hashers.bcrypt.cost"
	ViperKeyAdminListenOnHost              = "serve.admin.host"
	ViperKeyAdminListenOnPort              = "serve.admin.port"
	ViperKeyPublicListenOnHost             = "serve.public.host"
	ViperKeyPublicListenOnPort             = "serve.public.port"
	ViperKeyConsentRequestMaxAge           = "ttl.login_consent_request"
	ViperKeyAccessTokenLifespan            = "ttl.access_token"
	ViperKeyRefreshTokenLifespan           = "ttl.refresh_token"
	ViperKeyIDTokenLifespan                = "ttl.id_token"
	ViperKeyAuthCodeLifespan               = "ttl.auth_code"
	ViperKeyScopeStrategy                  = "strategies.scope"
	ViperKeyGetCookieSecrets               = "secrets.cookie"
	ViperKeyGetSystemSecret                = "secrets.system"
	ViperKeyLogoutRedirectURL              = "urls.post_logout_redirect"
	ViperKeyLoginURL                       = "urls.login"
	ViperKeyConsentURL                     = "urls.consent"
	ViperKeyErrorURL                       = "urls.error"
	ViperKeyPublicURL                      = "urls.self.public"
	ViperKeyIssuerURL                      = "urls.self.issuer"
	ViperKeyAllowTLSTerminationFrom        = "serve.tls.allow_termination_from"
	ViperKeyAccessTokenStrategy            = "strategies.access_token"
	ViperKeySubjectIdentifierAlgorithmSalt = "oidc.subject_identifiers.pairwise.salt"
)

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
}

func NewViperProvider(l logrus.FieldLogger, forcedHTTP bool, insecureRedirects []string) Provider {
	if insecureRedirects == nil {
		insecureRedirects = []string{}
	}
	return &ViperProvider{
		l:                 l,
		forcedHTTP:        forcedHTTP,
		insecureRedirects: insecureRedirects,
	}
}

func (v *ViperProvider) getAddress(address string, port int) string {
	if strings.HasPrefix(address, "unix:") {
		return address
	}
	return fmt.Sprintf("%s:%d", address, port)
}

func (v *ViperProvider) InsecureRedirects() []string {
	return v.insecureRedirects
}

func (v *ViperProvider) WellKnownKeys(include ...string) []string {
	if v.AccessTokenStrategy() == "jwt" {
		include = append(include, x.OpenIDConnectKeyName)
	}

	include = append(include, x.OpenIDConnectKeyName)
	return append(viperx.GetStringSlice(v.l, ViperKeyWellKnownKeys, []string{}), include...)
}

func (v *ViperProvider) ServesHTTPS() bool {
	return !v.forcedHTTP
}

func (v *ViperProvider) IsUsingJWTAsAccessTokens() bool {
	return v.AccessTokenStrategy() != "opaque"
}

func (v *ViperProvider) SubjectTypesSupported() []string {
	types := stringslice.Filter(
		viperx.GetStringSlice(v.l,
			ViperKeySubjectTypesSupported,
			[]string{"public"},
			"OIDC_SUBJECT_TYPES_SUPPORTED",
		),
		func(s string) bool {
			return !(s == "public" || s == "pairwise")
		},
	)

	if len(types) == 0 {
		types = []string{"public"}
	}

	if stringslice.Has(types, "pairwise") {
		if v.AccessTokenStrategy() == "jwt" {
			v.l.Fatalf(`The pairwise subject identifier algorithm is not supported by the JWT OAuth 2.0 Access Token Strategy. Please remove "pairwise" from oidc.subject_identifiers.supported or set strategies.access_token to "opaque".`)
		}
		if len(v.SubjectIdentifierAlgorithmSalt()) < 8 {
			v.l.Fatalf(`The pairwise subject identifier algorithm was set but length of oidc.subject_identifier.salt is too small (%d < 8), please set oidc.subject_identifiers.pairwise.salt to a random string with 8 characters or more.`, len(v.SubjectIdentifierAlgorithmSalt()))
		}
	}

	return types
}

func (v *ViperProvider) DefaultClientScope() []string {
	return viperx.GetStringSlice(v.l,
		ViperKeyDefaultClientScope,
		[]string{"offline_access", "offline", "openid"},
		"OIDC_DYNAMIC_CLIENT_REGISTRATION_DEFAULT_SCOPE",
	)
}

func (v *ViperProvider) CORSEnabled(iface string) bool {
	return corsx.IsEnabled(v.l, "serve."+iface)
}

func (v *ViperProvider) CORSOptions(iface string) cors.Options {
	return corsx.ParseOptions(v.l, "serve."+iface)
}

func (v *ViperProvider) DSN() string {
	return viperx.GetString(v.l, ViperKeyDSN, "", "DATABASE_URL")
}

func (v *ViperProvider) DataSourcePlugin() string {
	return viperx.GetString(v.l, ViperKeyDSN, "", "DATABASE_URL")
}

func (v *ViperProvider) BCryptCost() int {
	return viperx.GetInt(v.l, ViperKeyBCryptCost, 10, "BCRYPT_COST")
}

func (v *ViperProvider) AdminListenOn() string {
	host := viperx.GetString(v.l, ViperKeyAdminListenOnHost, "", "ADMIN_HOST")
	port := viperx.GetInt(v.l, ViperKeyAdminListenOnPort, 4445, "ADMIN_PORT")
	return v.getAddress(host, port)
}

func (v *ViperProvider) PublicListenOn() string {
	return v.getAddress(v.publicHost(), v.publicPort())
}

func (v *ViperProvider) publicHost() string {
	return viperx.GetString(v.l, ViperKeyPublicListenOnHost, "", "PUBLIC_HOST")
}

func (v *ViperProvider) publicPort() int {
	return viperx.GetInt(v.l, ViperKeyPublicListenOnPort, 4444, "PUBLIC_PORT")
}

func (v *ViperProvider) adminHost() string {
	return viperx.GetString(v.l, ViperKeyAdminListenOnHost, "", "ADMIN_HOST")
}

func (v *ViperProvider) adminPort() int {
	return viperx.GetInt(v.l, ViperKeyAdminListenOnPort, 4445, "ADMIN_PORT")
}

func (v *ViperProvider) ConsentRequestMaxAge() time.Duration {
	return viperx.GetDuration(v.l, ViperKeyConsentRequestMaxAge, time.Minute*30, "LOGIN_CONSENT_REQUEST_LIFESPAN")
}

func (v *ViperProvider) AccessTokenLifespan() time.Duration {
	return viperx.GetDuration(v.l, ViperKeyAccessTokenLifespan, time.Hour, "ACCESS_TOKEN_LIFESPAN")
}

func (v *ViperProvider) RefreshTokenLifespan() time.Duration {
	return viperx.GetDuration(v.l, ViperKeyRefreshTokenLifespan, time.Hour*720, "REFRESH_TOKEN_LIFESPAN")
}

func (v *ViperProvider) IDTokenLifespan() time.Duration {
	return viperx.GetDuration(v.l, ViperKeyIDTokenLifespan, time.Hour, "ID_TOKEN_LIFESPAN")
}

func (v *ViperProvider) AuthCodeLifespan() time.Duration {
	return viperx.GetDuration(v.l, ViperKeyAuthCodeLifespan, time.Minute*10, "AUTH_CODE_LIFESPAN")
}

func (v *ViperProvider) ScopeStrategy() string {
	return viperx.GetString(v.l, ViperKeyScopeStrategy, "", "SCOPE_STRATEGY")
}

func (v *ViperProvider) TracingServiceName() string {
	return viperx.GetString(v.l, "tracing.service_name", "ORY Hydra")
}

func (v *ViperProvider) TracingProvider() string {
	return viperx.GetString(v.l, "tracing.provider", "", "TRACING_PROVIDER")
}

func (v *ViperProvider) TracingJaegerConfig() *tracing.JaegerConfig {
	return &tracing.JaegerConfig{
		LocalAgentHostPort: viperx.GetString(v.l, "tracing.providers.jaeger.local_agent_address", "", "TRACING_PROVIDER_JAEGER_LOCAL_AGENT_ADDRESS"),
		SamplerType:        viperx.GetString(v.l, "tracing.providers.jaeger.sampling.type", "const", "TRACING_PROVIDER_JAEGER_SAMPLING_TYPE"),
		SamplerValue:       viperx.GetFloat64(v.l, "tracing.providers.jaeger.sampling.value", float64(1), "TRACING_PROVIDER_JAEGER_SAMPLING_VALUE"),
		SamplerServerURL:   viperx.GetString(v.l, "tracing.providers.jaeger.sampling.server_url", "", "TRACING_PROVIDER_JAEGER_SAMPLING_SERVER_URL"),
	}
}

func (v *ViperProvider) GetCookieSecrets() [][]byte {
	return [][]byte{
		[]byte(viperx.GetString(v.l, ViperKeyGetCookieSecrets, string(v.GetSystemSecret()), "COOKIE_SECRET")),
	}
}

func (v *ViperProvider) GetRotatedSystemSecrets() [][]byte {
	secrets := viperx.GetStringSlice(v.l, ViperKeyGetSystemSecret, []string{})

	if len(secrets) < 2 {
		return nil
	}

	var rotated [][]byte
	for _, secret := range secrets[1:] {
		rotated = append(rotated, x.HashStringSecret(secret))
	}

	return rotated
}

func (v *ViperProvider) GetSystemSecret() []byte {
	secrets := viperx.GetStringSlice(v.l, ViperKeyGetSystemSecret, []string{}, "SYSTEM_SECRET")

	if len(secrets) == 0 {
		if v.generatedSecret != nil {
			return v.generatedSecret
		}

		v.l.Warnf("Configuration secrets.system is not set, generating a temporary, random secret...")
		secret, err := x.GenerateSecret(32)
		cmdx.Must(err, "Could not generate secret: %s", err)

		v.l.Warnf("Generated secret: %s", secret)
		v.generatedSecret = x.HashByteSecret(secret)

		v.l.Warnln("Do not use generate secrets in production. The secret will be leaked to the logs.")
		return x.HashByteSecret(secret)
	}

	secret := secrets[0]
	if len(secret) >= 16 {
		return x.HashStringSecret(secret)
	}

	v.l.Fatalf("System secret must be undefined or have at least 16 characters but only has %d characters.", len(secret))
	return nil
}

func (v *ViperProvider) LogoutRedirectURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyLogoutRedirectURL, "", "OAUTH2_LOGOUT_REDIRECT_URL"))
}

func (v *ViperProvider) adminFallbackURL(path string) string {
	return v.fallbackURL(path, v.adminHost(), v.adminPort())

}

func (v *ViperProvider) publicFallbackURL(path string) string {
	if len(v.IssuerURL().String()) > 0 {
		return urlx.AppendPaths(v.IssuerURL(), path).String()
	}
	return v.fallbackURL(path, v.publicHost(), v.publicPort())
}

func (v *ViperProvider) fallbackURL(path string, host string, port int) string {
	var u url.URL
	u.Scheme = "https"
	if !v.ServesHTTPS() {
		u.Scheme = "http"
	}
	if host == "" {
		u.Host = fmt.Sprintf("%s:%d", "localhost", port)
	}
	u.Path = path
	return u.String()
}

func (v *ViperProvider) LoginURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyLoginURL, v.publicFallbackURL("oauth2/fallbacks/consent"), "OAUTH2_LOGIN_URL"))
}

func (v *ViperProvider) ConsentURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyConsentURL, v.publicFallbackURL("oauth2/fallbacks/consent"), "OAUTH2_CONSENT_URL"))
}

func (v *ViperProvider) ErrorURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyErrorURL, v.publicFallbackURL("oauth2/fallbacks/error"), "OAUTH2_ERROR_URL"))
}

func (v *ViperProvider) PublicURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyPublicURL, v.publicFallbackURL("/")))
}

func (v *ViperProvider) IssuerURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyIssuerURL, v.fallbackURL("", v.publicHost(), v.publicPort()), "OAUTH2_ISSUER_URL", "ISSUER", "ISSUER_URL"))
}

func (v *ViperProvider) OAuth2AuthURL() string {
	return "/oauth2/auth" // this should not have the host etc prepended...
}

func (v *ViperProvider) OAuth2ClientRegistrationURL() *url.URL {
	return urlx.ParseOrFatal(v.l, viperx.GetString(v.l, ViperKeyOAuth2ClientRegistrationURL, "", "OAUTH2_CLIENT_REGISTRATION_URL"))
}

func (v *ViperProvider) AllowTLSTerminationFrom() []string {
	return viperx.GetStringSlice(v.l, ViperKeyAllowTLSTerminationFrom, []string{}, "HTTPS_ALLOW_TERMINATION_FROM")
}

func (v *ViperProvider) AccessTokenStrategy() string {
	return strings.ToLower(viperx.GetString(v.l, ViperKeyAccessTokenStrategy, "opaque", "OAUTH2_ACCESS_TOKEN_STRATEGY"))
}

func (v *ViperProvider) SubjectIdentifierAlgorithmSalt() string {
	return viperx.GetString(v.l, ViperKeySubjectIdentifierAlgorithmSalt, "", "OIDC_SUBJECT_TYPE_PAIRWISE_SALT")
}

func (v *ViperProvider) OIDCDiscoverySupportedClaims() []string {
	return stringslice.Unique(
		append(
			[]string{"sub"},
			viperx.GetStringSlice(v.l, ViperKeyOIDCDiscoverySupportedClaims, []string{}, "OIDC_DISCOVERY_CLAIMS_SUPPORTED")...,
		),
	)
}

func (v *ViperProvider) OIDCDiscoverySupportedScope() []string {
	return stringslice.Unique(
		append(
			[]string{"offline", "openid"},
			viperx.GetStringSlice(v.l, ViperKeyOIDCDiscoverySupportedScope, []string{}, "OIDC_DISCOVERY_SCOPES_SUPPORTED")...,
		),
	)
}

func (v *ViperProvider) OIDCDiscoveryUserinfoEndpoint() string {
	return viperx.GetString(v.l, ViperKeyOIDCDiscoveryUserinfoEndpoint, urlx.AppendPaths(v.PublicURL(), "/userinfo").String(), "OIDC_DISCOVERY_USERINFO_ENDPOINT")
}

func (v *ViperProvider) ShareOAuth2Debug() bool {
	return viperx.GetBool(v.l, "oauth2.expose_internal_errors", "OAUTH2_SHARE_ERROR_DEBUG")
}
