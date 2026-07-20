# platform-api

Capa de borde (antes “UX”): recibe HTTP público (y mañana gRPC) de apps móviles/web,
traduce a DTOs/comandos y **solo habla con `platform-core`**. No toca la base de datos.

Arquitectura hexagonal: adapters de entrada (HTTP público) + adapter de salida (cliente HTTP/gRPC al core).

## Por qué este nombre

| Nombre tentativo | Nombre elegido | Motivo |
|------------------|----------------|--------|
| UX | **platform-api** | No es UI; es el edge/API gateway de producto |
| core | **platform-core** | Dominio + persistencia multi-tenant |

## Responsabilidad

- Auth/session de canales públicos (hooks listos; integrar Keycloak/JWT por cliente).
- Validación superficial de request + mapeo DTO ↔ contratos del core.
- Enrutamiento multi-tenant (`X-Tenant-Slug` o path).
- Agregar respuestas amigables para frontends.

## Layout

```
cmd/server/
internal/
  domain/ports/           # puertos (CoreClient, etc.)
  application/            # casos de borde (orquestación liviana)
  adapters/
    http/public/          # REST público
    http/middleware/
    coreclient/           # outbound → platform-core
  platform/               # wiring
```

## Arranque

```bash
# terminal 1: core
cd ../platform-core && go run -buildvcs=false ./cmd/server

# terminal 2: api (puerto 8082 si Keycloak usa 8080)
export CORE_BASE_URL=http://localhost:8083
export HTTP_ADDR=:8082
go run -buildvcs=false ./cmd/server
```

Con el stack nails, usa `fullstack-nails/scripts-mac/start-app.sh` (o Windows).

Público ejemplo:

```bash
curl -H 'X-Tenant-Slug: nails-demo' http://localhost:8082/v1/catalog/services
```

## Añadir un cliente nuevo (p.ej. barbería)

1. Crear tenant en core (`POST /v1/tenants`).
2. Apuntar la app del cliente a este API con `X-Tenant-Slug: barber-acme`.
3. Reutilizar los mismos endpoints genéricos (`bookings`, `payments`, `catalog`).
4. Branding/feature flags viven en config del API o en un servicio de configuración — no en el core.

## Relación con fullstack-nails

`fullstack-nails` es el producto (mobile + scripts). La API es este repo + `platform-core`.
El antiguo Express fue retirado.
