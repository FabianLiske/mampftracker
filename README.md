# MampfTracker

Ein schlanker, selbst gehosteter Ernährungstracker für Kalorien, Makro- und
Mikronährstoffe. Backend und Weboberfläche werden gemeinsam als einzelnes
Container-Image ausgeliefert. Persönliche Daten und bekannte Lebensmittel
liegen ausschließlich in einer lokalen SQLite-Datenbank.

## Funktionen

- Tagesprotokoll für Frühstück, Mittagessen, Abendessen und Snacks
- Tagesziele für Kalorien, Protein, Kohlenhydrate, Fett und Ballaststoffe
- Detailwerte für Zucker, gesättigte Fettsäuren, Salz und Mikronährstoffe
- Manuell angelegte Lebensmittel inklusive Mikronährstoffen
- Gespeicherte Standardmengen für Produkte und manuell angelegte Lebensmittel
- Optionaler Anzahl-Multiplikator, beispielsweise `2 × 80 g` für zwei Brezeln
- Barcode-Suche über Open Food Facts
- Kamera-Scanner in Browsern mit `BarcodeDetector`-Unterstützung
- Responsive Weboberfläche
- Optionaler Zugriffsschutz über HTTP Basic Auth

Open-Food-Facts-Daten sind Community-Daten und können unvollständig oder falsch
sein. Datensätze von [Open Food Facts](https://world.openfoodfacts.org/) stehen
unter der ODbL.

## Produktdaten und lokaler Cache

Ein Barcode wird nicht bei jeder Verwendung erneut extern abgefragt:

1. MampfTracker sucht den Barcode zuerst in der lokalen SQLite-Datenbank.
2. Ist das Produkt vorhanden, wird unmittelbar der lokale Datensatz verwendet.
3. Nur unbekannte Barcodes werden bei Open Food Facts abgefragt.
4. Ein erfolgreicher Treffer wird als vollständiges Lebensmittel lokal
   gespeichert und steht danach auch ohne erneute API-Anfrage zur Verfügung.
5. Beim ersten Eintragen legst du die gewünschte Grammzahl fest. Sie wird als
   Standardmenge gespeichert und bei späteren Einträgen automatisch eingesetzt.

Die Datenbank ist damit gleichzeitig Produktcache und Quelle für manuell
angelegte Lebensmittel. Bereits gespeicherte Produkte werden derzeit nicht
automatisch mit Open Food Facts synchronisiert oder überschrieben.

Bei manuell angelegten Lebensmitteln wird die Standardmenge direkt zusammen mit
den Nährwerten erfasst, beispielsweise 80 g für eine Brezel.

Lokale Korrekturen an einem bestehenden Produkt sind im aktuellen Stand noch
nicht über die Weboberfläche möglich. Die Datenstruktur ist dafür geeignet,
aber ein Bearbeiten-/Aktualisieren-Endpunkt und die zugehörige UI fehlen noch.

## Entwicklung

Frontend installieren und starten:

```bash
cd frontend
npm install
npm run dev
```

In einem zweiten Terminal das Backend starten:

```bash
go run ./cmd/server
```

Vite leitet `/api` automatisch an Port 8080 weiter. Die SQLite-Datei liegt dabei
standardmäßig unter `./data/mampftracker.db`.

Tests und Produktionsbuild:

```bash
go test ./...
cd frontend && npm run build
```

## Container-Image

Das Multi-Stage-Dockerfile baut zuerst das React-Frontend und bettet dessen
statische Dateien anschließend in das Go-Binary ein:

### Lokal mit Docker Compose

Voraussetzung ist lediglich eine laufende Docker-Engine mit Docker Compose.
Node.js, npm, Go und SQLite müssen lokal nicht installiert sein, da alle
Build-Schritte im Container stattfinden.

Aus dem Repository-Hauptverzeichnis:

```bash
docker compose up --build
```

Beim ersten Start werden die Node- und Go-Basisimages geladen, das Frontend
gebaut, das Go-Binary kompiliert und der Container gestartet. Anschließend ist
MampfTracker unter <http://localhost:8080> erreichbar.

Die SQLite-Datenbank liegt im benannten Docker-Volume `mampftracker-data` und
bleibt bei einem normalen Neustart oder Rebuild erhalten.

Nützliche Befehle:

```bash
# Im Hintergrund starten
docker compose up --build -d

# Status und Healthcheck anzeigen
docker compose ps

# Logs verfolgen
docker compose logs -f

# Container stoppen und entfernen, Daten behalten
docker compose down

# Container UND lokale Datenbank löschen
docker compose down -v
```

Nach Codeänderungen genügt erneut:

```bash
docker compose up --build -d
```

Falls Port 8080 bereits belegt ist, kann in `compose.yaml` beispielsweise
`"8081:8080"` eingetragen werden. Die Anwendung läuft dann unter
<http://localhost:8081>.

### Image direkt bauen

Alternativ kann das Image ohne Compose gebaut werden:

```bash
docker build -t mampftracker:local .
```

Ein lokaler Testlauf:

```bash
docker run --rm \
  -p 8080:8080 \
  -v mampftracker-data:/data \
  -e OPENFOODFACTS_USER_AGENT="MampfTracker/0.1 (self-hosted; kontakt@example.com)" \
  mampftracker:local
```

Danach ist die Anwendung unter <http://localhost:8080> erreichbar.

## Deployment-Vertrag für Flux/Kubernetes

Dieses Repository enthält bewusst keine Kubernetes- oder Flux-Manifeste.
Für das Deployment im separaten Flux-Repository gelten folgende Eckdaten.

### Container

| Eigenschaft | Wert |
| --- | --- |
| HTTP-Port | `8080` |
| Container-Benutzer | UID/GID `10001` |
| Persistentes Verzeichnis | `/data` |
| Standard-Datenbank | `/data/mampftracker.db` |
| Health-Endpunkt | `GET /api/health` |
| Health-Erfolg | HTTP `200` mit `{"status":"ok"}` |
| Benötigter Egress | HTTPS zu `world.openfoodfacts.org` |

Der Container kann mit `readOnlyRootFilesystem: true` laufen, sofern `/data`
schreibbar und `/tmp` beispielsweise über ein `emptyDir` verfügbar ist. Alle
Linux-Capabilities können entfernt werden; Privilege Escalation ist nicht
erforderlich.

### Persistenz und Replikate

- `/data` muss auf ein persistentes `ReadWriteOnce`-Volume gemountet werden.
- SQLite erlaubt für diesen Anwendungsfall genau eine schreibende Instanz.
- Deshalb `replicas: 1` und eine `Recreate`-Strategie verwenden.
- Mehrere gleichzeitig laufende Pods oder Rolling Updates mit Überlappung
  vermeiden.
- 1 GiB Speicher ist für eine persönliche Instanz normalerweise mehr als
  ausreichend.

SQLite läuft im WAL-Modus. Für konsistente Backups sollte bevorzugt ein
SQLite-Online-Backup verwendet werden. Bei dateibasierten Snapshots müssen die
Datenbankdatei und eventuell vorhandene Dateien mit den Endungen `-wal` und
`-shm` gemeinsam gesichert werden.

### Probes

Readiness- und Liveness-Probes können beide verwenden:

```text
GET /api/health
Port 8080
```

Der Health-Endpunkt bleibt auch bei aktivierter Basic Auth ohne Anmeldung
erreichbar.

### Ressourcen

Für eine persönliche Instanz sind folgende Startwerte angemessen:

```yaml
requests:
  cpu: 20m
  memory: 32Mi
limits:
  cpu: 500m
  memory: 256Mi
```

### Konfiguration

| Variable | Container-Standard | Beschreibung |
| --- | --- | --- |
| `PORT` | `8080` | HTTP-Port |
| `DATABASE_PATH` | `/data/mampftracker.db` | Pfad zur SQLite-Datenbank |
| `OPENFOODFACTS_USER_AGENT` | MampfTracker-Kennung | Kennung für externe API-Anfragen |
| `TZ` | Systemzeitzone | Zeitzone des Containers, beispielsweise `Europe/Berlin` |
| `AUTH_USERNAME` | leer | Optionaler Benutzername für HTTP Basic Auth |
| `AUTH_PASSWORD` | leer | Optionales Passwort für HTTP Basic Auth |

Basic Auth wird nur aktiviert, wenn Benutzername und Passwort gesetzt sind.
Beide Werte sollten im Flux-Repository aus einem Secret kommen. Für den
Open-Food-Facts-User-Agent sollte eine Kontaktadresse oder Projekt-URL angegeben
werden.

### Ingress

- Der Kamera-Scanner benötigt außerhalb von `localhost` HTTPS.
- Es ist kein spezielles WebSocket- oder Session-Stickiness-Setup nötig.
- Die Anwendung wird vollständig unter `/` ausgeliefert.
- Bei öffentlicher Erreichbarkeit sollte mindestens die eingebaute Basic Auth
  oder eine vorgeschaltete Authentifizierung verwendet werden.

## HTTP-API

- `GET /api/foods?q=...`
- `POST /api/foods`
- `PUT /api/foods/{id}/serving`
- `GET /api/foods/barcode/{code}`
- `GET /api/entries?date=YYYY-MM-DD`
- `POST /api/entries`
- `PUT /api/entries/{id}`
- `DELETE /api/entries/{id}`
- `GET|PUT /api/goals`
- `GET /api/health`
