# lockd-go2

A backend service for controlling remote locks via MQTT, featuring Access Control Lists (ACLs) and user-based permissions.

## Features
- **ACL Support**: Granular control over which authenticated users can see and operate specific locks.
- **MQTT Integration**: Communicates with locks via MQTT.
- **Secure API**: Requires API keys and provides audit logging.
- **Wildcard Permissions**: Support for `*` to grant full access to users or for specific locks.

## Installation

1. Install Go:
   ```bash
   apt-get install -y golang
   ```

2. Initialize and build:
   ```bash
   go mod init github.com/marcipain/lockd-go2
   go mod tidy
   go build -o lockd2 ./main.go
   ```

3. Install the binary:
   ```bash
   install -m 0755 lockd2 /usr/local/bin/lockd2
   ```

4. Setup configuration:
   ```bash
   mkdir -p /etc/lockd
   cp lockd2.json /etc/lockd/lockd2.json
   ```

5. Setup Systemd service (example):
   Create `/etc/systemd/system/lockd2.service`:
   ```ini
   [Unit]
   Description=Lockd2 Backend Service
   After=network.target

   [Service]
   Type=simple
   User=root
   ExecStart=/usr/local/bin/lockd2 -config /etc/lockd/lockd2.json
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target
   ```

6. Enable and start:
   ```bash
   systemctl daemon-reload
   systemctl enable --now lockd2
   systemctl status lockd2
   ```

## Configuration (lockd2.json)

The configuration file includes an `acl` section to manage permissions:

```json
{
    "locks": [
        { "id": "front", "name": "Front Door", "type": "TOGGLE", "has_battery": true },
        { "id": "gate", "name": "Gate", "type": "OPEN", "has_battery": false }
    ],
    "acl": [
        { "user": "admin", "locks": ["*"] },
        { "user": "marci", "locks": ["front", "gate"] }
    ]
}
```

- **`user`**: The username from your `auth_file`. Use `*` for all users.
- **`locks`**: List of IDs or `*` for all.

---

### HTTPS / TLS Configuration (ACME / Let's Encrypt)
To enable secure communication, add your certificate files to the `http` section:
```json
"http": {
    "listen": "0.0.0.0:443",
    "cert_file": "/etc/letsencrypt/live/domain.com/fullchain.pem",
    "key_file": "/etc/letsencrypt/live/domain.com/privkey.pem"
}
```
If both `cert_file` and `key_file` are provided, the server will start in **HTTPS** mode. Otherwise, it defaults to plain **HTTP**.

#### Automatic Certificate Renewal (ACME/Certbot)
If you use Certbot, you can automatically reload the certificates without restarting the server by adding a `post-hook`:
```bash
certbot renew --post-hook "pkill -HUP lockd2"
```
The server handles the `SIGHUP` signal to reload both the configuration and the TLS certificates from disk.

## Lock Types

The system supports three lock types to match different hardware:

1.  **`TOGGLE`**: Standard smart locks (e.g., front doors).
    - Supports `LOCK` and `UNLOCK`.
    - Shows "Open" or "Closed" status.
    - Mobile App: Shows two buttons (**NYIT** / **ZÁR**).
2.  **`STRIKE`**: Electric strikes (e.g., intercom latches).
    - Supports a single pulse command (internally `UNLOCK`).
    - Behavior: Merely releases the latch for a few seconds.
    - Mobile App: Shows one button (**NYITÁS** / **OPEN**).
3.  **`PULSE`**: Gate or garage door openers (pulse-based toggle).
    - Supports a single pulse command (internally `UNLOCK`).
    - Behavior: The same button is used to both start opening and start closing.
    - Mobile App: Shows one button (**GOMB** / **TRIGGER**).

## Configuration Helpers

The `lockd2` binary includes tools to help you set up the configuration file (`lockd2.json`):

### Base64 Encoding
MQTT brokers often require Base64 encoded credentials in the config. Use the `-encode` flag to generate them:
```bash
lockd2 -encode "your_password"
# Output: eW91cl9wYXNzd29yZA==
```
Copy the output into the `username` or `password` fields of your `lockd2.json`.

---

## Generating API Keys
Use the built-in tool to generate a new key for a user:
```bash
lockd2 -gen-key marci
```

### Step-by-Step Example:
1. **Run the command**: `lockd2 -gen-key marci`
2. **Copy the "Raw Key"**: It looks like a long string of numbers and letters. Paste this into the **Lockd2 Mobile App** (or Windows app) when it asks for the API Key.
3. **Copy the "Auth Line"**: It looks like `marci:a1b2c3d4...`.
4. **Update the server**: Open your `auth_keys` file (usually `/etc/lockd/auth_keys`) and paste the **Auth Line** at the end of the file on a new line.
5. **Reload (Optional)**: If the server is already running, send a `SIGHUP` signal to reload the keys: `pkill -HUP lockd2`.

**Important:**
- If you haven't created the `auth_keys` file yet, the server will create it automatically (empty) upon first run.

## Client App
This backend is designed to work with the **[Lockd2 Mobile App](https://github.com/MarciPain/lockd2)**. The app will automatically sync the list of locks based on the ACL permissions defined for the user.
