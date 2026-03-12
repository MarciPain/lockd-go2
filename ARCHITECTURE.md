# Projekt Architektúra - lockd-go2

## Aktuális állapot
A `lockd-go2` egy Go nyelven írt backend szolgáltatás, amely központi vezérlőként funkcionál a Lockd ökoszisztémában. Feladata az MQTT alapú eszközzár-vezérlés, a felhasználói jogosultságok kezelése (ACL) és az API kulcs alapú hitelesítés biztosítása a kliensek (mobilapp, addon) számára.

## Funkcionalitás
- **MQTT Vezérlés**: Kapcsolattartás a hardveres egységekkel.
- **ACL (Access Control List)**: Felhasználónkénti szabályozás, hogy ki melyik zárat láthatja és vezérelheti.
- **API Kulcs Management**: Biztonságos hozzáférés a kliensalkalmazások számára.
- **SIGHUP Kezelés**: Konfiguráció és TLS tanúsítványok menet közbeni újratöltése.

## Fájllista és funkciók
- [main.go](./main.go): A teljes szerver logika, beleértve a HTTP API-t, MQTT klienst és ACL logikát.
- [lockd2.json](./lockd2.json): Konfigurációs fájl (zárak definiálása, MQTT paraméterek, ACL szabályok).
- [auth_keys](file:///etc/lockd/auth_keys): (Létrehozandó/Meglévő) API kulcsokat és hozzájuk tartozó felhasználókat tartalmazó fájl.

## Kapcsolódó Projektek
- [lockd2 Mobilapp](https://github.com/MarciPain/lockd2): Flutter alapú kliens.
- [hass-lockd2-addon](https://github.com/MarciPain/hass-lockd2-addon): Home Assistant integráció.
