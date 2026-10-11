# Línea base de OpenSpec

Estas specs describen el comportamiento observable de la versión revisada del repositorio. Se incorporaron directamente a `openspec/specs/` como inventario inicial; no representan una propuesta de funcionalidad nueva. Cada requisito tiene al menos un escenario verificable. Las correcciones futuras deben usar deltas en `openspec/changes/` y sincronizarse al archivar.

| Capacidad | Fuente principal |
| --- | --- |
| `configuration-and-runtime` | `cmd/`, `server/main.go`, `worker/main.go`, `server/config/`, `worker/config/` |
| `job-intake` | `server/web/web.go`, `server/scheduler/scheduler.go`, `model/model.go` |
| `job-dispatch` | `server/scheduler/scheduler.go`, `worker/worker/coordinator.go`, `worker/worker/executor.go` |
| `events-and-progress` | `server/scheduler/scheduler.go`, `server/repository/repository.go`, `worker/worker/progress.go` |
| `media-transfer` | `server/web/web.go`, `server/scheduler/streams.go`, `worker/step/downloadStep.go`, `worker/step/uploadStep.go` |
| `media-encoding` | `worker/ffmpeg/ffprobe.go`, `worker/step/ffmpegStep.go`, `worker/step/ffmpegVerifyStep.go` |
| `subtitles` | `worker/ffmpeg/ffprobe.go`, `worker/step/MKVExtractStep.go`, `worker/step/PGStoSrtStep.go` |
| `persistence` | `server/repository/repository.go`, `server/repository/resources/database/` |
| `maintenance-and-recovery` | `server/scheduler/scheduler.go`, `worker/job/job.go`, `worker/worker/executor.go` |
| `self-update` | `update/update.go`, `server/web/web.go`, `worker/worker/coordinator.go` |
| `distribution` | `Makefile`, `Dockerfile`, `version/`, `.github/workflows/` |

## Desviaciones conocidas

Estas observaciones no se convierten en requisitos normativos. Son puntos a corregir o aclarar mediante cambios posteriores, con pruebas específicas:

- `CancelJob` añade `Job/canceled` a trabajos `assigned` o `started`, pero después devuelve un error de estado desconocido; la API puede responder HTTP 500 pese a haber registrado la cancelación. Los trabajos `queued` tampoco se cancelan por esta ruta.
- Las rutas de descarga, checksum y subida no usan el middleware de Bearer aplicado a las cuatro rutas de control. Descarga y subida comprueban `workerName` y asignación; checksum solo busca el UUID y la suma disponible.
- Una subida inválida responde HTTP 400, pero el cierre diferido del stream renombra el archivo `.upload` al destino incluso tras el fallo; la limpieza puede eliminar el destino previo. No hay garantía de preservación del archivo anterior.
- La fase de codificación no asigna `Target.Size` al contexto. La comparación de tamaños de `ffmpegVerifyStep.go` no verifica efectivamente si la salida creció.
- Cuando se configura una ventana `startAfter`/`stopAfter`, `AcceptJobs` decide por horario y deja de aplicar el límite ordinario de dos trabajos activos.
- `skipped` figura en la respuesta de incorporación, pero el planificador actual clasifica los archivos no elegibles en `failed`; no produce `ErrorFileSkipped` en este flujo.

## Comprobación

La estructura de esta línea base se valida con `openspec validate --all --strict`. Los tests unitarios cubren partes del planificador, análisis de medios, contexto y pasos; las pruebas de integración con PostgreSQL y el E2E Docker están bajo el tag `integration`. La validación estructural no prueba por sí sola el comportamiento del sistema: cada modificación futura requiere pruebas correspondientes al escenario afectado.
