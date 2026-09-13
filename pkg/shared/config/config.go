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
	Site  Site
	Email Email
	SMTP  SMTP
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
// Called by NewServerConfig, which every consumer already calls, so a
// deployment that changes nothing behaves exactly as it did when these
// variables were read in 37 scattered places. That default matters more here
// than in the sibling libraries: this one is the foundation, three consumers
// mount into it, and a host that forgot to configure it would send every
// password reset from noreply@localhost with nothing to warn them.
//
// A host that wants to supply its own — a different bucket per consumer, a test
// harness with no environment at all — calls Set BEFORE NewServerConfig and
// this does nothing.
func EnsureDefault() {
	mu.Lock()
	defer mu.Unlock()
	if installed {
		return
	}
	current = FromEnv()
	installed = true
}

// Current returns the installed configuration.
func Current() Config {
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
