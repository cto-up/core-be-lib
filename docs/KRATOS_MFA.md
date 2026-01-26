# Optional MFA with Ory Kratos

Each user will be able to opt-in

Strategy,Web (Browser),Mobile (Native App),Key Technical Difference
TOTP,✅ Full Support,✅ Full Support,"Web: User scans QR. Mobile: App can use ""Deep Links"" to open the Authenticator app automatically."
Lookup Secrets,✅ Full Support,✅ Full Support,Purely text-based; works anywhere you can render a list of codes.
Recovery (Email),✅ Full Support,✅ Full Support,Kratos sends a link/code; works everywhere.
Passkeys (WebAuthn),✅ Full Support,⚠️ Limited/Complex,"Web: Standard browser API. Mobile: Native apps need a specialized ""Native API"" bridge (Kratos support for this is newer/experimental)."

## Architecture Overview

Sample archecture:

- **Vue.js (Cloudflare)** → Frontend handling UI flows
- **Golang Backend** → API server, Kratos SDK integration
- **Ory Kratos** → Identity & authentication
- **Nginx** → Reverse proxy for Kratos

## 1. Kratos Configuration

### Update `kratos.yml`

```yaml
selfservice:
  methods:
    totp:
      enabled: true
      config:
        issuer: YourAppName
    webauthn:
      enabled: true
      config:
        rp:
          display_name: YourAppName
          id: yourdomain.com # Without protocol
          origin: https://yourdomain.com
    lookup_secret:
      enabled: true

  flows:
    settings:
      ui_url: https://yourapp.com/settings
      privileged_session_max_age: 15m
      required_aal: aal1 # Allow AAL1 by default

    login:
      after:
        default_browser_return_url: https://yourapp.com/dashboard
      # AAL2 challenge when needed

session:
  whoami:
    required_aal: aal1 # Default session level
  lifespan: 720h

  earliest_possible_extend: 1h
  cookie:
    same_site: Lax
    domain: yourdomain.com
```

Key points:

- Enable TOTP, WebAuthn, and recovery codes (lookup_secret)
- Set `required_aal: aal1` to make MFA optional
- Configure privileged sessions for settings changes

## 3. Golang Backend Implementation

### Setup Kratos SDK

```go
// config/kratos.go
package config

import (
    "github.com/ory/kratos-client-go"
)

var KratosClient *kratos.APIClient

func InitKratos() {
    configuration := kratos.NewConfiguration()
    configuration.Servers = []kratos.ServerConfiguration{
        {
            URL: "http://localhost:4434", // Admin API (internal only)
        },
    }
    KratosClient = kratos.NewAPIClient(configuration)
}
```

### MFA Status Endpoint

```go
// handlers/mfa.go
package handlers

import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "your-app/config"
)

type MFAStatus struct {
    TOTPEnabled      bool     `json:"totp_enabled"`
    WebAuthnEnabled  bool     `json:"webauthn_enabled"`
    RecoveryCodesSet bool     `json:"recovery_codes_set"`
    AvailableMethods []string `json:"available_methods"`
}

func GetMFAStatus(c *gin.Context) {
    sessionCookie, err := c.Cookie("ory_kratos_session")
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
        return
    }

    // Get session info
    session, resp, err := config.KratosClient.FrontendApi.ToSession(context.Background()).
        Cookie(sessionCookie).
        Execute()

    if err != nil || resp.StatusCode != 200 {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid session"})
        return
    }

    identity := session.Identity
    status := MFAStatus{
        TOTPEnabled:      false,
        WebAuthnEnabled:  false,
        RecoveryCodesSet: false,
        AvailableMethods: []string{"totp", "webauthn", "lookup_secret"},
    }

    // Check credentials
    if credentials, ok := identity.Credentials["totp"]; ok {
        status.TOTPEnabled = credentials.GetConfig() != nil
    }

    if credentials, ok := identity.Credentials["webauthn"]; ok {
        config := credentials.GetConfig()
        if config != nil {
            if configMap, ok := config.(map[string]interface{}); ok {
                if creds, ok := configMap["credentials"].([]interface{}); ok {
                    status.WebAuthnEnabled = len(creds) > 0
                }
            }
        }
    }

    if credentials, ok := identity.Credentials["lookup_secret"]; ok {
        status.RecoveryCodesSet = credentials.GetConfig() != nil
    }

    c.JSON(http.StatusOK, status)
}
```

### Settings Flow Initialization

```go
func InitializeSettingsFlow(c *gin.Context) {
    sessionCookie, err := c.Cookie("ory_kratos_session")
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
        return
    }

    // Create settings flow
    flow, resp, err := config.KratosClient.FrontendApi.CreateNativeSettingsFlow(context.Background()).
        Cookie(sessionCookie).
        Execute()

    if err != nil || resp.StatusCode != 200 {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create settings flow"})
        return
    }

    c.JSON(http.StatusOK, flow)
}
```

### AAL2 Protection Middleware

```go
// middleware/aal.go
package middleware

import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "your-app/config"
)

func RequireAAL2() gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionCookie, err := c.Cookie("ory_kratos_session")
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{
                "error": "authentication_required",
            })
            c.Abort()
            return
        }

        session, _, err := config.KratosClient.FrontendApi.ToSession(context.Background()).
            Cookie(sessionCookie).
            Execute()

        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{
                "error": "invalid_session",
            })
            c.Abort()
            return
        }

        // Check AAL level
        if session.AuthenticatorAssuranceLevel != "aal2" {
            c.JSON(http.StatusForbidden, gin.H{
                "error": "mfa_required",
                "message": "This action requires MFA verification",
                "require_aal2": true,
            })
            c.Abort()
            return
        }

        c.Next()
    }
}
```

### Routes Setup

```go
// main.go or routes setup
func setupRoutes(r *gin.Engine) {
    api := r.Group("/api")
    {
        // Public endpoints
        api.GET("/mfa/status", handlers.GetMFAStatus)
        api.POST("/settings/init", handlers.InitializeSettingsFlow)

        // Protected endpoints requiring AAL2
        protected := api.Group("/protected")
        protected.Use(middleware.RequireAAL2())
        {
            protected.POST("/sensitive-action", handlers.SensitiveAction)
            protected.DELETE("/account", handlers.DeleteAccount)
        }
    }
}
```

## 4. Vue.js Frontend Implementation

### Kratos Service

```javascript
// services/kratosService.js
import axios from "axios";

const KRATOS_PUBLIC_URL = "https://auth.yourdomain.com";
const BACKEND_URL = "https://api.yourdomain.com";

export default {
  async getMFAStatus() {
    const response = await axios.get(`${BACKEND_URL}/api/mfa/status`, {
      withCredentials: true,
    });
    return response.data;
  },

  async initSettingsFlow() {
    const response = await axios.post(
      `${BACKEND_URL}/api/settings/init`,
      {},
      {
        withCredentials: true,
      },
    );
    return response.data;
  },

  async getSettingsFlow(flowId) {
    const response = await axios.get(
      `${KRATOS_PUBLIC_URL}/self-service/settings/flows`,
      {
        params: { id: flowId },
        withCredentials: true,
      },
    );
    return response.data;
  },

  async submitSettingsFlow(flowId, method, values) {
    const response = await axios.post(
      `${KRATOS_PUBLIC_URL}/self-service/settings`,
      {
        method,
        ...values,
      },
      {
        params: { flow: flowId },
        withCredentials: true,
      },
    );
    return response.data;
  },

  async initLoginFlow() {
    const response = await axios.get(
      `${KRATOS_PUBLIC_URL}/self-service/login/browser`,
      {
        withCredentials: true,
      },
    );
    return response.data;
  },

  async submitLoginFlow(flowId, method, values) {
    const response = await axios.post(
      `${KRATOS_PUBLIC_URL}/self-service/login`,
      {
        method,
        ...values,
      },
      {
        params: { flow: flowId },
        withCredentials: true,
      },
    );
    return response.data;
  },
};
```

### MFA Settings Component

```vue
<!-- components/MFASettings.vue -->
<template>
  <div class="mfa-settings">
    <h2>Multi-Factor Authentication</h2>

    <div v-if="loading" class="loading">Loading MFA status...</div>

    <div v-else class="mfa-methods">
      <!-- TOTP Section -->
      <div class="method-card">
        <div class="method-header">
          <h3>Authenticator App (TOTP)</h3>
          <span
            :class="['status', mfaStatus.totp_enabled ? 'enabled' : 'disabled']"
          >
            {{ mfaStatus.totp_enabled ? "Enabled" : "Disabled" }}
          </span>
        </div>
        <p>Use apps like Google Authenticator, Authy, or 1Password</p>

        <button
          v-if="!mfaStatus.totp_enabled"
          @click="setupTOTP"
          class="btn-primary"
        >
          Set Up Authenticator App
        </button>
        <button v-else @click="disableTOTP" class="btn-danger">
          Disable TOTP
        </button>
      </div>

      <!-- WebAuthn Section -->
      <div class="method-card">
        <div class="method-header">
          <h3>Security Key (WebAuthn)</h3>
          <span
            :class="[
              'status',
              mfaStatus.webauthn_enabled ? 'enabled' : 'disabled',
            ]"
          >
            {{ mfaStatus.webauthn_enabled ? "Enabled" : "Disabled" }}
          </span>
        </div>
        <p>Use hardware keys like YubiKey or built-in biometrics</p>

        <button
          v-if="!mfaStatus.webauthn_enabled"
          @click="setupWebAuthn"
          class="btn-primary"
        >
          Set Up Security Key
        </button>
        <button v-else @click="manageWebAuthn" class="btn-secondary">
          Manage Keys
        </button>
      </div>

      <!-- Recovery Codes -->
      <div
        class="method-card"
        v-if="mfaStatus.totp_enabled || mfaStatus.webauthn_enabled"
      >
        <div class="method-header">
          <h3>Recovery Codes</h3>
          <span
            :class="[
              'status',
              mfaStatus.recovery_codes_set ? 'enabled' : 'disabled',
            ]"
          >
            {{ mfaStatus.recovery_codes_set ? "Generated" : "Not Generated" }}
          </span>
        </div>
        <p>Use these codes if you lose access to your MFA device</p>

        <button @click="generateRecoveryCodes" class="btn-secondary">
          {{
            mfaStatus.recovery_codes_set ? "Regenerate Codes" : "Generate Codes"
          }}
        </button>
      </div>
    </div>

    <!-- TOTP Setup Modal -->
    <div v-if="showTOTPSetup" class="modal">
      <div class="modal-content">
        <h3>Set Up Authenticator App</h3>

        <div v-if="totpSetupStep === 'qr'">
          <p>Scan this QR code with your authenticator app:</p>
          <div class="qr-code" v-html="totpQRCode"></div>
          <p class="secret-key">
            Or enter this key manually: <code>{{ totpSecretKey }}</code>
          </p>
        </div>

        <div v-if="totpSetupStep === 'verify'">
          <p>Enter the 6-digit code from your authenticator app:</p>
          <input
            v-model="totpCode"
            type="text"
            maxlength="6"
            placeholder="000000"
            @input="validateTOTPInput"
          />
          <p v-if="totpError" class="error">{{ totpError }}</p>
        </div>

        <div class="modal-actions">
          <button @click="cancelTOTPSetup" class="btn-secondary">Cancel</button>
          <button
            v-if="totpSetupStep === 'qr'"
            @click="totpSetupStep = 'verify'"
            class="btn-primary"
          >
            Next
          </button>
          <button
            v-if="totpSetupStep === 'verify'"
            @click="verifyTOTP"
            class="btn-primary"
            :disabled="totpCode.length !== 6"
          >
            Verify & Enable
          </button>
        </div>
      </div>
    </div>

    <!-- Recovery Codes Modal -->
    <div v-if="showRecoveryCodes" class="modal">
      <div class="modal-content">
        <h3>Recovery Codes</h3>
        <div class="warning">
          <p>
            <strong>Important:</strong> Save these codes in a safe place. Each
            code can only be used once.
          </p>
        </div>

        <div class="recovery-codes">
          <code v-for="code in recoveryCodes" :key="code">{{ code }}</code>
        </div>

        <div class="modal-actions">
          <button @click="downloadRecoveryCodes" class="btn-secondary">
            Download
          </button>
          <button @click="copyRecoveryCodes" class="btn-secondary">Copy</button>
          <button @click="showRecoveryCodes = false" class="btn-primary">
            I've Saved Them
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import kratosService from "@/services/kratosService";

export default {
  name: "MFASettings",

  data() {
    return {
      loading: true,
      mfaStatus: {
        totp_enabled: false,
        webauthn_enabled: false,
        recovery_codes_set: false,
      },

      showTOTPSetup: false,
      totpSetupStep: "qr", // 'qr' or 'verify'
      totpQRCode: "",
      totpSecretKey: "",
      totpCode: "",
      totpError: "",
      settingsFlowId: null,

      showRecoveryCodes: false,
      recoveryCodes: [],
    };
  },

  async mounted() {
    await this.loadMFAStatus();
  },

  methods: {
    async loadMFAStatus() {
      try {
        this.loading = true;
        this.mfaStatus = await kratosService.getMFAStatus();
      } catch (error) {
        console.error("Failed to load MFA status:", error);
        this.$emit("error", "Failed to load MFA settings");
      } finally {
        this.loading = false;
      }
    },

    async setupTOTP() {
      try {
        // Initialize settings flow
        const flow = await kratosService.initSettingsFlow();
        this.settingsFlowId = flow.id;

        // Get TOTP setup data from flow
        const totpNode = this.findNodeByGroup(flow.ui.nodes, "totp");
        if (totpNode) {
          this.totpQRCode = totpNode.attributes.text?.text || "";
          this.totpSecretKey = this.extractSecretFromQR(this.totpQRCode);
        }

        this.showTOTPSetup = true;
        this.totpSetupStep = "qr";
      } catch (error) {
        console.error("Failed to setup TOTP:", error);
        this.$emit("error", "Failed to initialize TOTP setup");
      }
    },

    validateTOTPInput() {
      this.totpCode = this.totpCode.replace(/\D/g, "").slice(0, 6);
      this.totpError = "";
    },

    async verifyTOTP() {
      try {
        await kratosService.submitSettingsFlow(this.settingsFlowId, "totp", {
          totp_code: this.totpCode,
        });

        this.showTOTPSetup = false;
        this.totpCode = "";
        await this.loadMFAStatus();
        this.$emit("success", "TOTP enabled successfully");

        // Prompt for recovery codes
        await this.generateRecoveryCodes();
      } catch (error) {
        this.totpError = "Invalid code. Please try again.";
        console.error("TOTP verification failed:", error);
      }
    },

    cancelTOTPSetup() {
      this.showTOTPSetup = false;
      this.totpCode = "";
      this.totpError = "";
      this.totpSetupStep = "qr";
    },

    async disableTOTP() {
      if (!confirm("Are you sure you want to disable TOTP authentication?"))
        return;

      try {
        const flow = await kratosService.initSettingsFlow();
        await kratosService.submitSettingsFlow(flow.id, "totp", {
          totp_unlink: true,
        });

        await this.loadMFAStatus();
        this.$emit("success", "TOTP disabled successfully");
      } catch (error) {
        console.error("Failed to disable TOTP:", error);
        this.$emit("error", "Failed to disable TOTP");
      }
    },

    async setupWebAuthn() {
      try {
        const flow = await kratosService.initSettingsFlow();
        this.settingsFlowId = flow.id;

        // Trigger WebAuthn registration
        await kratosService.submitSettingsFlow(flow.id, "webauthn", {
          webauthn_register: true,
        });

        await this.loadMFAStatus();
        this.$emit("success", "Security key registered successfully");
      } catch (error) {
        console.error("WebAuthn setup failed:", error);
        this.$emit("error", "Failed to register security key");
      }
    },

    async generateRecoveryCodes() {
      try {
        const flow = await kratosService.initSettingsFlow();
        const response = await kratosService.submitSettingsFlow(
          flow.id,
          "lookup_secret",
          { lookup_secret_confirm: true },
        );

        // Extract recovery codes from response
        this.recoveryCodes = this.extractRecoveryCodes(response);
        this.showRecoveryCodes = true;
        await this.loadMFAStatus();
      } catch (error) {
        console.error("Failed to generate recovery codes:", error);
        this.$emit("error", "Failed to generate recovery codes");
      }
    },

    downloadRecoveryCodes() {
      const content = this.recoveryCodes.join("\n");
      const blob = new Blob([content], { type: "text/plain" });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "recovery-codes.txt";
      a.click();
      window.URL.revokeObjectURL(url);
    },

    async copyRecoveryCodes() {
      const content = this.recoveryCodes.join("\n");
      await navigator.clipboard.writeText(content);
      this.$emit("success", "Recovery codes copied to clipboard");
    },

    // Helper methods
    findNodeByGroup(nodes, group) {
      return nodes.find((node) => node.group === group);
    },

    extractSecretFromQR(qrText) {
      const match = qrText.match(/secret=([A-Z0-9]+)/);
      return match ? match[1] : "";
    },

    extractRecoveryCodes(response) {
      // Parse recovery codes from Kratos response
      // This depends on your Kratos configuration
      const node = response.ui.nodes.find(
        (n) => n.attributes.name === "lookup_secret_codes",
      );
      return node?.attributes.text?.text?.split(",") || [];
    },
  },
};
</script>

<style scoped>
.mfa-settings {
  max-width: 800px;
  margin: 0 auto;
  padding: 20px;
}

.mfa-methods {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.method-card {
  border: 1px solid #ddd;
  border-radius: 8px;
  padding: 20px;
  background: white;
}

.method-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.status {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
}

.status.enabled {
  background: #d4edda;
  color: #155724;
}

.status.disabled {
  background: #f8d7da;
  color: #721c24;
}

.modal {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  background: white;
  padding: 30px;
  border-radius: 8px;
  max-width: 500px;
  width: 90%;
}

.qr-code {
  text-align: center;
  margin: 20px 0;
}

.secret-key {
  text-align: center;
  margin: 10px 0;
}

.recovery-codes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin: 20px 0;
}

.recovery-codes code {
  background: #f5f5f5;
  padding: 8px;
  border-radius: 4px;
  text-align: center;
  font-family: monospace;
}

.warning {
  background: #fff3cd;
  border: 1px solid #ffc107;
  border-radius: 4px;
  padding: 15px;
  margin-bottom: 20px;
}

.modal-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  margin-top: 20px;
}

button {
  padding: 10px 20px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
}

.btn-primary {
  background: #007bff;
  color: white;
}

.btn-secondary {
  background: #6c757d;
  color: white;
}

.btn-danger {
  background: #dc3545;
  color: white;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  color: #dc3545;
  font-size: 14px;
  margin-top: 5px;
}
</style>
```

### Login Component with MFA Challenge

```vue
<!-- components/Login.vue -->
<template>
  <div class="login-container">
    <h2>Sign In</h2>

    <!-- Initial Login -->
    <form v-if="loginStep === 'credentials'" @submit.prevent="submitLogin">
      <input v-model="identifier" type="email" placeholder="Email" required />
      <input
        v-model="password"
        type="password"
        placeholder="Password"
        required
      />
      <button type="submit" class="btn-primary">Sign In</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>

    <!-- MFA Challenge -->
    <div v-if="loginStep === 'mfa'" class="mfa-challenge">
      <h3>Two-Factor Authentication</h3>
      <p>Enter your authentication code</p>

      <div class="mfa-method-selector">
        <button
          v-if="availableMFAMethods.includes('totp')"
          @click="selectedMFAMethod = 'totp'"
          :class="['method-btn', { active: selectedMFAMethod === 'totp' }]"
        >
          Authenticator App
        </button>
        <button
          v-if="availableMFAMethods.includes('webauthn')"
          @click="selectedMFAMethod = 'webauthn'"
          :class="['method-btn', { active: selectedMFAMethod === 'webauthn' }]"
        >
          Security Key
        </button>
        <button
          v-if="availableMFAMethods.includes('lookup_secret')"
          @click="selectedMFAMethod = 'lookup_secret'"
          :class="[
            'method-btn',
            { active: selectedMFAMethod === 'lookup_secret' },
          ]"
        >
          Recovery Code
        </button>
      </div>

      <div v-if="selectedMFAMethod === 'totp'" class="mfa-input">
        <input
          v-model="mfaCode"
          type="text"
          maxlength="6"
          placeholder="000000"
          @input="validateMFAInput"
        />
        <button
          @click="submitMFA"
          class="btn-primary"
          :disabled="mfaCode.length !== 6"
        >
          Verify
        </button>
      </div>

      <div v-if="selectedMFAMethod === 'webauthn'" class="mfa-webauthn">
        <p>Insert your security key and press the button</p>
        <button @click="submitWebAuthn" class="btn-primary">
          Use Security Key
        </button>
      </div>

      <div v-if="selectedMFAMethod === 'lookup_secret'" class="mfa-input">
        <input
          v-model="recoveryCode"
          type="text"
          placeholder="Enter recovery code"
        />
        <button @click="submitRecoveryCode" class="btn-primary">
          Use Recovery Code
        </button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>

<script>
import kratosService from "@/services/kratosService";

export default {
  name: "Login",

  data() {
    return {
      loginStep: "credentials", // 'credentials' or 'mfa'
      identifier: "",
      password: "",
      mfaCode: "",
      recoveryCode: "",
      selectedMFAMethod: "totp",
      availableMFAMethods: [],
      flowId: null,
      error: "",
    };
  },

  methods: {
    async submitLogin() {
      try {
        this.error = "";

        // Initialize login flow if not exists
        if (!this.flowId) {
          const flow = await kratosService.initLoginFlow();
          this.flowId = flow.id;
        }

        // Submit password
        const response = await kratosService.submitLoginFlow(
          this.flowId,
          "password",
          {
            identifier: this.identifier,
            password: this.password,
          },
        );

        // Check if MFA is required
        if (response.ui?.messages?.some((m) => m.id === 1010004)) {
          // AAL2 required - show MFA challenge
          this.loginStep = "mfa";
          this.detectAvailableMFAMethods(response);
        } else {
          // Login successful
          this.$router.push("/dashboard");
        }
      } catch (error) {
        this.error = "Invalid email or password";
        console.error("Login failed:", error);
      }
    },

    async submitMFA() {
      try {
        this.error = "";

        await kratosService.submitLoginFlow(this.flowId, "totp", {
          totp_code: this.mfaCode,
        });

        this.$router.push("/dashboard");
      } catch (error) {
        this.error = "Invalid authentication code";
        console.error("MFA verification failed:", error);
      }
    },

    async submitWebAuthn() {
      try {
        this.error = "";

        await kratosService.submitLoginFlow(this.flowId, "webauthn", {
          webauthn_login: true,
        });

        this.$router.push("/dashboard");
      } catch (error) {
        this.error = "Security key verification failed";
        console.error("WebAuthn failed:", error);
      }
    },

    async submitRecoveryCode() {
      try {
        this.error = "";

        await kratosService.submitLoginFlow(this.flowId, "lookup_secret", {
          lookup_secret: this.recoveryCode,
        });

        this.$router.push("/dashboard");
      } catch (error) {
        this.error = "Invalid recovery code";
        console.error("Recovery code failed:", error);
      }
    },

    validateMFAInput() {
      this.mfaCode = this.mfaCode.replace(/\D/g, "").slice(0, 6);
    },

    detectAvailableMFAMethods(response) {
      // Parse available methods from flow UI
      const nodes = response.ui.nodes || [];
      this.availableMFAMethods = [];

      if (nodes.some((n) => n.group === "totp")) {
        this.availableMFAMethods.push("totp");
      }
      if (nodes.some((n) => n.group === "webauthn")) {
        this.availableMFAMethods.push("webauthn");
      }
      if (nodes.some((n) => n.group === "lookup_secret")) {
        this.availableMFAMethods.push("lookup_secret");
      }

      this.selectedMFAMethod = this.availableMFAMethods[0];
    },
  },
};
</script>
```

## 5. Key Implementation Points

### Database Considerations

Kratos stores all MFA credentials internally. You don't need to manage:

- TOTP secrets
- WebAuthn credentials
- Recovery codes

### Session Management

- **AAL1**: Password-only authentication
- **AAL2**: Password + MFA authentication
- Sessions automatically track AAL level
- Use `required_aal` in flows to enforce MFA for specific actions

### Security Best Practices

1. **Always use HTTPS** - MFA is meaningless without it
2. **Implement rate limiting** on Nginx for auth endpoints
3. **Monitor failed MFA attempts** via Kratos logs
4. **Backup recovery codes** - educate users to save them
5. **Session timeout** - privileged sessions for settings changes

### Testing Checklist

- [ ] User can enable TOTP and scan QR code
- [ ] User can verify TOTP code
- [ ] Login requires MFA after enabling
- [ ] User can enable WebAuthn
- [ ] Recovery codes work when MFA device is lost
- [ ] User can disable MFA methods
- [ ] AAL2 middleware protects sensitive endpoints
- [ ] Session cookies work across Vue (Cloudflare) and backend

This gives you a complete, production-ready MFA implementation where users have full control over whether to use MFA and which methods to enable.
