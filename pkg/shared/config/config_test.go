package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/shared/config"
)

// The From: address of every password reset, welcome mail, verification and
// membership invitation used to fall back to a hardcoded noreply@ctoup.com —
// the vendor's own domain — in NINE separate copies of the same three lines.
//
// Any deployment that did not set SYSTEM_EMAIL signed its users' mail with
// somebody else's brand, in a library sold on sovereign hosting and per-tenant
// isolation. aiemployee-lib had the identical bug with a different literal and
// fixed it; this is the same fix, nine times over.

func TestSystemEmailPrefersTheConfiguredSender(t *testing.T) {
	config.Set(config.Config{
		Email: config.Email{SystemFrom: "alerts@acme.test"},
		Site:  config.Site{Domain: "other.test"},
	})
	t.Cleanup(func() { config.Set(config.Config{}) })

	assert.Equal(t, "alerts@acme.test", config.SystemEmailFrom())
}

func TestSystemEmailDerivesFromTheDeploymentsOwnDomain(t *testing.T) {
	t.Cleanup(func() { config.Set(config.Config{}) })

	for name, tc := range map[string]struct{ domain, want string }{
		"bare":               {"acme.test", "noreply@acme.test"},
		"scheme stripped":    {"https://acme.test", "noreply@acme.test"},
		"trailing slash":     {"https://acme.test/", "noreply@acme.test"},
		"port stripped":      {"ctoup.localhost:5173", "noreply@ctoup.localhost"},
		"whitespace ignored": {"  acme.test  ", "noreply@acme.test"},
		"trailing dot":       {"acme.test.", "noreply@acme.test"},
	} {
		t.Run(name, func(t *testing.T) {
			config.Set(config.Config{Site: config.Site{Domain: tc.domain}})
			assert.Equal(t, tc.want, config.SystemEmailFrom())
		})
	}
}

// A misconfigured deployment gets a non-routable placeholder. Mail that bounces
// is a visible problem; mail signed with a real third party's domain is an
// invisible one.
func TestSystemEmailFallsBackToSomethingNonRoutable(t *testing.T) {
	config.Set(config.Config{})
	assert.Equal(t, "noreply@localhost", config.SystemEmailFrom())
}

// The specific regression, stated as the invariant rather than as one literal:
// whatever the configuration, the sender comes from this deployment's own
// domain or from an explicit setting — never from a name compiled into the
// library.
func TestSystemEmailNeverBorrowsAnotherBrand(t *testing.T) {
	t.Cleanup(func() { config.Set(config.Config{}) })

	for _, domain := range []string{"", "acme.test", "https://tenant.example.org/"} {
		config.Set(config.Config{Site: config.Site{Domain: domain}})
		got := config.SystemEmailFrom()
		assert.NotContains(t, got, "ctoup", "got %q for domain %q", got, domain)
		assert.NotContains(t, got, "sparkmeee", "got %q for domain %q", got, domain)
	}
}

// FromEnv keeps every variable's existing name, so adopting this costs no
// deployment a change.
func TestFromEnvKeepsTheNamesDeploymentsAlreadyUse(t *testing.T) {
	t.Setenv("DOMAIN", "acme.test")
	t.Setenv("SYSTEM_EMAIL", "alerts@acme.test")
	t.Setenv("SMTP_HOST", "smtp.acme.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "secret")

	c := config.FromEnv()
	assert.Equal(t, "acme.test", c.Site.Domain)
	assert.Equal(t, "alerts@acme.test", c.Email.SystemFrom)
	assert.Equal(t, "smtp.acme.test", c.SMTP.Host)
	assert.Equal(t, "587", c.SMTP.Port)
	assert.Equal(t, "mailer", c.SMTP.Username)
	assert.Equal(t, "secret", c.SMTP.Password)
}

// The guard. core-be-lib is the library every other one mounts into, and it
// read the environment in more places than any of its siblings while
// tooling-lib and workflow-lib read it zero times.
//
//	37 reads across 12 packages  ->  2, both deliberate
//
// This covers pkg/ — the LIBRARY surface. cmd/ and internal/ are core-be-lib's
// own application, where reading the environment is the host's job.
func TestTheLibraryDoesNotReadTheEnvironment(t *testing.T) {
	exempt := map[string]string{
		// The environment strategy itself: named, opt-in, and the one place a
		// read belongs.
		"shared/config/from_env.go": "FromEnv is the strategy",

		// Google's ADC convention. The blob client resolves ADC by itself; the
		// read here only parses the same file for its project_id. Routing it
		// through Config would mean re-implementing credential discovery the
		// SDK already does, and every other tool in that ecosystem reads this
		// variable the same way.
		"shared/fileservice/file_service.go": "GOOGLE_APPLICATION_CREDENTIALS is Google's ADC convention",
	}

	getenv := regexp.MustCompile(`os\.Getenv\(|os\.LookupEnv\(`)

	root := "../.."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for suffix, why := range exempt {
			if strings.HasSuffix(rel, suffix) {
				t.Logf("exempt: %s — %s", rel, why)
				return nil
			}
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if getenv.MatchString(line) {
				t.Errorf("%s reads the environment: %s\n\nThe host supplies configuration "+
					"through config.Set, and config.FromEnv is the strategy that reads it. "+
					"If this read is genuinely a third-party convention rather than a "+
					"deployment decision, add it to the exemption list WITH a reason.",
					rel, strings.TrimSpace(line))
			}
		}
		return nil
	})
	require.NoError(t, err)
}
