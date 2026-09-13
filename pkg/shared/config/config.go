// Package config is what the DEPLOYMENT decides, supplied by the host.
//
// core-be-lib is the foundation every other library mounts into, and it read
// os.Getenv in 37 places across 12 packages — more than any of its siblings,
// while tooling-lib and workflow-lib read it zero times. The library with the
// most consumers and the most security-relevant surface had the weakest
// guarantees, which is the wrong way round.
//
// What reading the environment inside a library costs:
//
//   - the host's env documentation cannot be authoritative, because the
//     variables are read inside a dependency;
//   - three consumers (hub, lms, care) share one process environment, so none
//     can point at a different bucket, SMTP relay or Kratos without changing it
//     for all three;
//   - tests reach for t.Setenv, which serialises them and leaks across
//     packages — and this library has the least test coverage of the four.
//
// What it does NOT cost, and this is worth stating because an earlier draft of
// the audit got it wrong: none of these belong in the tenant vault. ADR 027 is
// about credentials that vary BY TENANT — a provider API key, where the answer
// is "this tenant's, then the platform's". There is no per-tenant database
// password. Env is the right SOURCE for all of them; the only question this
// package settles is who reads it.
//
// FromEnv is the env-reading strategy, named and opt-in, so a host adopts it in
// one line and the library itself contains no hidden reads.
package config

import (
	"strings"
	"sync"
)

// Config is the deployment's answer for everything this library needs from its
// environment. Grouped by what each governs, not by which file reads it.
type Config struct {
	Site     Site
	Email    Email
	SMTP     SMTP
	Database Database
	Kratos   Kratos
	Seed     Seed
	Sentry   Sentry
	Turn     Turn
	Storage  Storage
}

// Database is the one Postgres this process talks to. Required: there is no
// sensible default for somebody else's database, so the caller fails loudly
// rather than connecting somewhere unintended.
type Database struct {
	URL      string // host:port/dbname?opts — no scheme, no credentials
	Username string
	Password string
}

// Kratos is the identity provider. The defaults are its documented local-dev
// ports, which is why a developer needs to set neither.
type Kratos struct {
	AdminURL  string // zero means http://localhost:4434
	PublicURL string // zero means http://localhost:4433
}

// Seed is the first administrator a fresh deployment is given. BOTH must be
// present or seeding is skipped — half a credential is not an account.
type Seed struct {
	UserEmail    string
	UserPassword string
}

// Sentry is error and trace reporting. Empty DSN disables it entirely, which is
// the correct default for a local run.
type Sentry struct {
	DSN              string
	Environment      string
	Release          string
	TracesSampleRate string
}

// Turn is the STUN/TURN server for realtime media.
type Turn struct {
	PublicIP string
	Port     string
}

// Storage is where uploaded files live.
//
// Provider selects the backend; empty means the local folder. The credentials
// here are NOT a general credential store: each cloud SDK discovers its own
// (Google's ADC, the AWS chain), and routing those through this struct would
// mean re-implementing discovery the SDKs already do. Azure is the exception —
// its shared key is built by hand rather than discovered.
type Storage struct {
	Provider  string // "gcs" | "s3" | "azure" | "" (local folder)
	Bucket    string // GCS/S3 bucket or Azure container
	LocalPath string // when Provider is ""
	AWSRegion string

	AzureAccount string
	AzureKey     string

	// BootstrapBucket creates the bucket or container if it is absent.
	//
	// Off by default, and that is a CHANGE: this used to happen on every boot,
	// unconditionally, from two handler constructors — so a deployment whose
	// storage is provisioned by terraform made two doomed round trips and
	// logged two permission errors at every start. A deployment that wants the
	// convenience asks for it.
	BootstrapBucket bool
}

// Site is who this deployment IS: the domain its URLs and addresses derive
// from.
type Site struct {
	// Domain is the bare apex this deployment serves, e.g. "taskfactory.cloud".
	// Empty is a misconfiguration rather than a mode — see Email.SystemFrom.
	Domain string
}

// Email is who this deployment's system mail comes FROM.
type Email struct {
	// SystemFrom is the sender for password resets, welcome mail, verification
	// and membership invitations. Empty derives noreply@<Site.Domain>.
	//
	// This used to be read in NINE places, each falling back to a hardcoded
	// noreply@ctoup.com — a domain belonging to the vendor, in a library sold
	// on sovereign hosting and per-tenant isolation. Any deployment that did
	// not set SYSTEM_EMAIL signed its users' mail with somebody else's brand.
	// The fallback is now derived from the deployment's own domain, and where
	// there is no domain either it is a non-routable placeholder, never a real
	// third party.
	SystemFrom string
}

// SMTP is the relay that actually sends it.
type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
}

// FallbackSystemEmail is what a deployment that has configured neither a sender
// nor a domain sends from. Deliberately non-routable: mail that bounces is a
// visible misconfiguration, and mail signed with a real third party's domain is
// an invisible one.
const FallbackSystemEmail = "noreply@localhost"

// Kratos' documented local-dev ports, so a developer configures neither.
const (
	DefaultKratosAdminURL  = "http://localhost:4434"
	DefaultKratosPublicURL = "http://localhost:4433"
)

var (
	mu        sync.RWMutex
	current   Config
	installed bool
)

// Set installs the deployment's configuration. A host calls this once, before
// anything reads a value — typically config.Set(config.FromEnv()).
func Set(c Config) {
	mu.Lock()
	defer mu.Unlock()
	current = c
	installed = true
}

// EnsureDefault installs FromEnv() if no host has called Set.
//
// A deployment that changes nothing behaves exactly as it did when these
// variables were read in 37 scattered places. That default matters more here
// than in the sibling libraries: this one is the foundation, three consumers
// mount into it, and a host that forgot to configure it would connect to no
// database and send every password reset from noreply@localhost, with nothing
// to warn anybody.
//
// A host that wants to supply its own — a different bucket per consumer, a test
// harness with no environment at all — calls Set first and this does nothing.
func EnsureDefault() {
	mu.Lock()
	defer mu.Unlock()
	if installed {
		return
	}
	current = FromEnv()
	installed = true
}

// Current returns the installed configuration, installing the environment
// default on the first read if no host has called Set.
//
// The default is applied HERE rather than at a single wiring hook, and that is
// the whole design. The first version installed it in NewServerConfig, which
// every consumer calls — but the database connection is opened BEFORE that, so
// the very first read found an empty Config and the process died on
// "DATABASE_USERNAME required". A foundation library cannot assume the order
// its callers reach it in; the value has to be there whenever somebody asks.
func Current() Config {
	mu.RLock()
	if installed {
		defer mu.RUnlock()
		return current
	}
	mu.RUnlock()

	EnsureDefault()

	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Domain is the deployment's apex, normalised to a bare hostname: no scheme, no
// port, no trailing dots. Empty when nobody told us.
func Domain() string {
	d := strings.TrimSpace(Current().Site.Domain)
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimRight(d, "/")
	if host, _, found := strings.Cut(d, ":"); found {
		d = host
	}
	return strings.Trim(d, ".")
}

// SystemEmailFrom is the sender address for this deployment's system mail.
//
// One function, where there were nine copies of the same three lines. Falls
// back to the deployment's own domain, and only then to a non-routable
// placeholder — never to a domain this library's vendor happens to own.
func SystemEmailFrom() string {
	if from := strings.TrimSpace(Current().Email.SystemFrom); from != "" {
		return from
	}
	if d := Domain(); d != "" {
		return "noreply@" + d
	}
	return FallbackSystemEmail
}

// SMTPSettings is the relay configuration.
func SMTPSettings() SMTP { return Current().SMTP }

// DatabaseSettings is the Postgres connection.
func DatabaseSettings() Database { return Current().Database }

// KratosSettings is the identity provider, with its local-dev defaults applied.
func KratosSettings() Kratos {
	k := Current().Kratos
	if k.AdminURL == "" {
		k.AdminURL = DefaultKratosAdminURL
	}
	if k.PublicURL == "" {
		k.PublicURL = DefaultKratosPublicURL
	}
	return k
}

// SeedSettings is the first administrator, if one was configured.
func SeedSettings() Seed { return Current().Seed }

// SentrySettings is the error reporter.
func SentrySettings() Sentry { return Current().Sentry }

// TurnSettings is the STUN/TURN server.
func TurnSettings() Turn { return Current().Turn }

// StorageSettings is where uploaded files live.
func StorageSettings() Storage { return Current().Storage }
