# MampfTracker – Deployment

Container-Image:

```text
ghcr.io/fabianliske/mampftracker:latest
```

Verfügbare Plattformen: `linux/amd64` und `linux/arm64`.

## Konfiguration

| Variable | Standard | Beschreibung |
| --- | --- | --- |
| `PORT` | `8080` | HTTP-Port |
| `DATABASE_PATH` | `/data/mampftracker.db` | SQLite-Datenbank |
| `TZ` | Systemzeitzone | Beispielsweise `Europe/Berlin` |
| `OPENFOODFACTS_USER_AGENT` | MampfTracker-Kennung | Kennung für Open-Food-Facts-Anfragen |
| `AUTH_USERNAME` | leer | Optionaler Benutzername für Basic Auth |
| `AUTH_PASSWORD` | leer | Optionales Passwort für Basic Auth |

Persistente Daten liegen unter `/data`. Der Health-Endpunkt ist
`GET /api/health` auf Port `8080`.

## Standalone-Container

```bash
docker volume create mampftracker-data

docker run -d \
  --name mampftracker \
  --restart unless-stopped \
  -p 8080:8080 \
  -v mampftracker-data:/data \
  -e TZ=Europe/Berlin \
  -e 'OPENFOODFACTS_USER_AGENT=MampfTracker/0.1 (self-hosted; kontakt@example.com)' \
  ghcr.io/fabianliske/mampftracker:latest
```

Optional mit Basic Auth:

```bash
docker run -d \
  --name mampftracker \
  --restart unless-stopped \
  -p 8080:8080 \
  -v mampftracker-data:/data \
  -e AUTH_USERNAME=admin \
  -e AUTH_PASSWORD=bitte-aendern \
  ghcr.io/fabianliske/mampftracker:latest
```

## Docker Compose

Die enthaltene `compose.yaml` baut das Image lokal:

```bash
docker compose up --build -d
docker compose logs -f
```

Stoppen, Daten behalten:

```bash
docker compose down
```

Stoppen und Volume löschen:

```bash
docker compose down -v
```

Für das GHCR-Image kann der Service stattdessen so definiert werden:

```yaml
services:
  mampftracker:
    image: ghcr.io/fabianliske/mampftracker:latest
    ports:
      - "8080:8080"
    environment:
      TZ: Europe/Berlin
      OPENFOODFACTS_USER_AGENT: "MampfTracker/0.1 (self-hosted)"
    volumes:
      - mampftracker-data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/api/health"]
      interval: 10s
      timeout: 3s
      retries: 3
      start_period: 5s
    restart: unless-stopped

volumes:
  mampftracker-data:
```

## Kubernetes

Wichtige Laufzeiteigenschaften:

- genau ein Replica, da SQLite verwendet wird
- Deployment-Strategie `Recreate`
- persistentes `ReadWriteOnce`-Volume auf `/data`
- Container-Port `8080`
- Container läuft als UID/GID `10001`
- Egress per HTTPS zu `world.openfoodfacts.org`
- HTTPS am Ingress für den Kamera-Scanner

Beispiel:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mampftracker-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mampftracker
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: mampftracker
  template:
    metadata:
      labels:
        app: mampftracker
    spec:
      imagePullSecrets:
        - name: ghcr-cred
      securityContext:
        fsGroup: 10001
      containers:
        - name: mampftracker
          image: ghcr.io/fabianliske/mampftracker:latest
          ports:
            - name: http
              containerPort: 8080
          env:
            - name: DATABASE_PATH
              value: /data/mampftracker.db
            - name: TZ
              value: Europe/Berlin
            - name: OPENFOODFACTS_USER_AGENT
              value: "MampfTracker/0.1 (self-hosted)"
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            readOnlyRootFilesystem: true
            runAsNonRoot: true
            runAsUser: 10001
            runAsGroup: 10001
          readinessProbe:
            httpGet:
              path: /api/health
              port: http
          livenessProbe:
            httpGet:
              path: /api/health
              port: http
          volumeMounts:
            - name: data
              mountPath: /data
            - name: tmp
              mountPath: /tmp
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: mampftracker-data
        - name: tmp
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: mampftracker
spec:
  selector:
    app: mampftracker
  ports:
    - port: 80
      targetPort: http
```

Das Root-Dateisystem kann read-only sein, wenn `/data` und `/tmp` schreibbar
gemountet werden.

Für konsistente Backups sollte ein SQLite-Online-Backup oder ein
Volume-Snapshot verwendet werden. Bei dateibasierten Backups müssen die
Datenbank sowie gegebenenfalls vorhandene `-wal`- und `-shm`-Dateien gemeinsam
gesichert werden.
