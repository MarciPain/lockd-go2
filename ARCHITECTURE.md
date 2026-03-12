# Project Architecture - lockd-go2

## Current State
`lockd-go2` is a backend service written in Go, acting as the central controller in the Lockd ecosystem. Its tasks include MQTT-based device lock control, user permission management (ACL), and providing API key-based authentication for clients (mobile app, addon).

## Functionality
- **MQTT Control**: Communication with hardware units.
- **ACL (Access Control List)**: Per-user regulation of which locks can be seen and controlled.
- **API Key Management**: Secure access for client applications.
- **SIGHUP Handling**: On-the-fly reloading of configuration and TLS certificates.

## File List and Functions
- [main.go](./main.go): The entire server logic, including HTTP API, MQTT client, and ACL logic.
- [lockd2.json](./lockd2.json): Configuration file (defining locks, MQTT parameters, ACL rules).
- [auth_keys](file:///etc/lockd/auth_keys): File containing API keys and their associated users (existent or to be created).

## Related Projects
- [lockd2 Mobile App](https://github.com/MarciPain/lockd2): Flutter-based client.
- [hass-lockd2-addon](https://github.com/MarciPain/hass-lockd2-addon): Home Assistant integration.

---

[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-Donate-orange.svg)](https://buymeacoffee.com/marcipain)
