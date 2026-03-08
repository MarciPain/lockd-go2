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

## Generating API Keys
Use the built-in tool:
```bash
lockd2 -gen-key <username>
```
Follow the output instructions to add the hash to your auth file.
