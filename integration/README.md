# Tests de Integración

Este directorio contiene tests de integración que usan **PostgreSQL real** con testcontainers.

## 📋 Tests Disponibles

### TestServerWorkerIntegration

Test de integración completo que simula el flujo completo de encoding:

1. **PostgreSQL con Testcontainers**
   - Levanta PostgreSQL real en container Docker
   - Inicializa el schema de la base de datos
   - Usa repositorio real (no mock)

2. **Setup**
   - Crea directorios temporales
   - Genera un archivo de video de prueba
   - Configura el scheduler con repositorio real

3. **Scheduling**
   - Programa un job de encoding
   - Verifica que se cree correctamente en PostgreSQL

4. **Worker Request**
   - Simula un worker solicitando un job
   - Verifica que se asigne correctamente
   - Valida el estado del job (Assigned)

5. **Job Execution**
   - Worker reporta inicio del job
   - Simula progreso de download (0%, 25%, 50%, 75%, 100%)
   - Simula encoding con ffmpeg
   - Simula upload del resultado
   - Reporta cada fase completada

6. **Completion**
   - Worker reporta job completado
   - Verifica estado final (Completed)
   - Valida que el archivo target existe
   - Verifica tamaños y compresión

7. **Verification**
   - Valida historial completo de eventos en PostgreSQL
   - Verifica secuencia correcta de estados

## 🐳 Testcontainers

Este test usa [testcontainers-go](https://golang.testcontainers.org/) para levantar PostgreSQL real:

### Ventajas
- ✅ **PostgreSQL real** - No mock, base de datos real
- ✅ **Aislamiento** - Cada test tiene su propia BD
- ✅ **Limpieza automática** - Container se elimina al terminar
- ✅ **Realistic** - Prueba queries SQL reales
- ✅ **CI-ready** - Funciona en GitHub Actions con Docker

### Requisitos
- Docker instalado y corriendo
- Puerto de PostgreSQL disponible

## 🚀 Cómo Ejecutar

### Ejecutar todos los tests de integración

```bash
make test-integration
```

### Ejecutar con timeout largo
```bash
go test -tags=integration ./integration/... -v -timeout 5m
```

### Ejecutar test específico
```bash
go test -tags=integration ./integration/... -v -run TestServerWorkerIntegration
```

### Ver output detallado
```bash
go test -tags=integration ./integration/... -v -count=1
```

## 📊 Qué Cubre Este Test

### Componentes Testeados

- ✅ **PostgreSQL Real**
  - Schema completo
  - Queries reales
  - Transacciones
  - Indices

- ✅ **Repository**
  - CRUD operations
  - Event storage
  - State queries
  - Transaction handling

- ✅ **Scheduler**
  - Programación de jobs
  - Asignación a workers
  - Manejo de eventos
  - Completado de jobs

- ✅ **Flujo Completo**
  - Estado: Queued → Assigned → Started → Completed
  - Eventos de progreso
  - Notificaciones por fase
  - Actualización de metadatos

### Estados Verificados

1. `QueuedNotificationStatus` - Job en cola
2. `AssignedNotificationStatus` - Job asignado a worker
3. `StartedNotificationStatus` - Job iniciado
4. `CompletedNotificationStatus` - Job completado

### Eventos Verificados

1. **JobNotification** - Estados del job principal
2. **DownloadNotification** - Descarga del archivo fuente
3. **FFMPEGSNotification** - Encoding con ffmpeg
4. **UploadNotification** - Subida del resultado
5. **ProgressEvent** - Eventos de progreso

## 🏗️ Arquitectura del Test

```
┌─────────────────────────────────────────────┐
│         Test de Integración                 │
│  (TestServerWorkerIntegration)             │
└───────────────┬─────────────────────────────┘
                │
                ├─► 🐳 PostgreSQL (Testcontainer)
                │   └─► Base de datos real
                │       ├─► Schema completo
                │       ├─► Queries reales
                │       └─► Transacciones
                │
                ├─► Repository (REAL)
                │   └─► Conexión a PostgreSQL
                │
                ├─► Scheduler (Real)
                │   └─► Maneja lógica de negocio
                │
                ├─► Worker (Simulado)
                │   └─► Genera eventos como worker real
                │
                └─► Filesystem (Temporal)
                    └─► Archivos reales en tmpDir
```

## 📁 Archivos

- `server_worker_test.go` - Test de integración principal
- `simple_test.go` - Test de verificación básico
- `README.md` - Esta documentación

## 🎯 Ventajas de Este Test

1. **PostgreSQL Real**
   - Prueba queries SQL reales
   - Verifica constraints y foreign keys
   - Detecta problemas de migrations
   - Testing realista

2. **Aislamiento Completo**
   - Cada test tiene su propia BD
   - Sin conflictos entre tests
   - Limpieza automática

3. **CI/CD Ready**
   - Funciona en GitHub Actions
   - No requiere PostgreSQL pre-instalado
   - Docker es suficiente

4. **Determinista**
   - Estado inicial limpio
   - Sin datos residuales
   - Resultados consistentes

5. **Coverage Alto**
   - Cubre flujo completo
   - Todos los estados
   - Manejo de errores
   - Queries complejas

## 🔍 Output Esperado

```
=== RUN   TestServerWorkerIntegration
    🐳 Starting PostgreSQL container...
    ✅ PostgreSQL started: postgresql://test:test@localhost:54321/transcoderd_test?sslmode=disable (port: 54321)
    ✅ Repository initialized
    ✅ Test files created
    ✅ Scheduler created
=== RUN   TestServerWorkerIntegration/Schedule_Job
    ✅ Job scheduled: 123e4567-e89b-12d3-a456-426614174000
=== RUN   TestServerWorkerIntegration/Request_Job
    ✅ Job assigned to worker: 123e4567-e89b-12d3-a456-426614174000 (EventID: 2)
=== RUN   TestServerWorkerIntegration/Worker_Starts_Job
    ✅ Job started by worker
=== RUN   TestServerWorkerIntegration/Simulate_Encoding_Process
=== RUN   TestServerWorkerIntegration/Simulate_Encoding_Process/Download_Progress
    ✅ Download progress simulated
    ✅ Download completed
    ✅ Encoding completed
    ✅ Upload completed
=== RUN   TestServerWorkerIntegration/Complete_Job
    ✅ Job completed successfully!
       Source: test-video.mkv (1048576 bytes)
       Target: test-video_encoded.mkv (524288 bytes)
       Compression: 50.00%
=== RUN   TestServerWorkerIntegration/Verify_Event_History
    ✅ Event history verified (7 events)
       1. Job - queued
       2. Job - assigned
       3. Job - started
       4. Download - completed
       5. FFMPEG - completed
       6. Upload - completed
       7. Job - completed
--- PASS: TestServerWorkerIntegration (5.50s)
PASS
```

## 🚧 Requisitos del Sistema

### Docker
```bash
# Verificar que Docker está corriendo
docker ps

# Si no está corriendo
# macOS: Abrir Docker Desktop
# Linux: sudo systemctl start docker
```

### Go Modules
```bash
# Las dependencias se instalan automáticamente
go mod download
```

## 📝 Dependencias

```go
github.com/testcontainers/testcontainers-go
github.com/testcontainers/testcontainers-go/modules/postgres
```

## 🚧 Troubleshooting

### Error: "Cannot connect to Docker daemon"
```bash
# Verifica que Docker está corriendo
docker ps

# En macOS, abre Docker Desktop
open -a Docker
```

### Error: "Port already in use"
```bash
# Testcontainers usa puertos aleatorios, esto no debería pasar
# Si ocurre, verifica que no hay containers huérfanos:
docker ps -a
docker rm -f $(docker ps -aq)
```

### Test muy lento
```bash
# Primera ejecución descarga la imagen de PostgreSQL
# Ejecuciones posteriores son más rápidas (~5-10 segundos)

# Pre-descargar la imagen:
docker pull postgres:16-alpine
```

## 📈 Tiempo de Ejecución

- **Primera vez:** ~30-60 segundos (descarga imagen PostgreSQL)
- **Ejecuciones posteriores:** ~5-10 segundos
- La mayoría del tiempo es startup del container

## 🎓 Comparación: Mock vs Testcontainers

| Aspecto | Mock en Memoria | Testcontainers |
|---------|----------------|----------------|
| Velocidad | ⚡ Muy rápido (~0.5s) | 🐢 Más lento (~5-10s) |
| Realismo | 🎭 Simulado | ✅ PostgreSQL real |
| SQL Testing | ❌ No | ✅ Sí |
| Setup | 🎯 Simple | 🐳 Requiere Docker |
| CI/CD | ✅ Siempre funciona | ✅ Funciona con Docker |
| Aislamiento | ⚠️ Por código | ✅ Por container |
| Debugging | 🔍 Fácil | 🔍 Medio |

## 💡 Conclusión

**Usamos testcontainers porque:**
- Tests más **realistas** con PostgreSQL real
- Detecta **bugs de SQL** que un mock no encontraría
- **Confidence** alta de que funciona en producción
- Vale la pena los ~5 segundos extra de ejecución


## 📋 Tests Disponibles

### TestServerWorkerIntegration

Test de integración completo que simula el flujo completo de encoding:

1. **Setup**
   - Crea directorios temporales
   - Genera un archivo de video de prueba
   - Configura el scheduler con repositorio en memoria

2. **Scheduling**
   - Programa un job de encoding
   - Verifica que se cree correctamente

3. **Worker Request**
   - Simula un worker solicitando un job
   - Verifica que se asigne correctamente
   - Valida el estado del job (Assigned)

4. **Job Execution**
   - Worker reporta inicio del job
   - Simula progreso de download (0%, 25%, 50%, 75%, 100%)
   - Simula encoding con ffmpeg
   - Simula upload del resultado
   - Reporta cada fase completada

5. **Completion**
   - Worker reporta job completado
   - Verifica estado final (Completed)
   - Valida que el archivo target existe
   - Verifica tamaños y compresión

6. **Verification**
   - Valida historial completo de eventos
   - Verifica secuencia correcta de estados

## 🚀 Cómo Ejecutar

### Ejecutar todos los tests de integración

```bash
go test -tags=integration ./integration/... -v
```

### Ejecutar con timeout largo
```bash
go test -tags=integration ./integration/... -v -timeout 5m
```

### Ejecutar test específico
```bash
go test -tags=integration ./integration/... -v -run TestServerWorkerIntegration
```

### Ver output detallado
```bash
go test -tags=integration ./integration/... -v -count=1
```

## 📊 Qué Cubre Este Test

### Componentes Testeados

- ✅ **Scheduler**
  - Programación de jobs
  - Asignación a workers
  - Manejo de eventos
  - Completado de jobs

- ✅ **Repository (Mock)**
  - Almacenamiento de jobs
  - Queries por estado
  - Manejo de eventos
  - Actualización de jobs

- ✅ **Flujo Completo**
  - Estado: Queued → Assigned → Started → Completed
  - Eventos de progreso
  - Notificaciones por fase
  - Actualización de metadatos

### Estados Verificados

1. `QueuedNotificationStatus` - Job en cola
2. `AssignedNotificationStatus` - Job asignado a worker
3. `StartedNotificationStatus` - Job iniciado
4. `CompletedNotificationStatus` - Job completado

### Eventos Verificados

1. **JobNotification** - Estados del job principal
2. **DownloadNotification** - Descarga del archivo fuente
3. **FFMPEGSNotification** - Encoding con ffmpeg
4. **UploadNotification** - Subida del resultado
5. **ProgressEvent** - Eventos de progreso

## 🏗️ Arquitectura del Test

```
┌─────────────────────────────────────────────┐
│         Test de Integración                 │
│  (TestServerWorkerIntegration)             │
└───────────────┬─────────────────────────────┘
                │
                ├─► Scheduler (Real)
                │   └─► Maneja lógica de negocio
                │
                ├─► Repository (Mock en Memoria)
                │   └─► Simula base de datos
                │
                ├─► Worker (Simulado)
                │   └─► Genera eventos como worker real
                │
                └─► Filesystem (Temporal)
                    └─► Archivos reales en tmpDir
```

## 📁 Archivos

- `server_worker_test.go` - Test de integración principal
- `repository_mock.go` - Implementación en memoria de Repository
- `README.md` - Esta documentación

## 🎯 Ventajas de Este Test

1. **Whitebox Testing**
   - Acceso completo al estado interno
   - Verificación detallada de cada paso
   - Debugging más fácil

2. **Sin Dependencias Externas**
   - No necesita PostgreSQL
   - No necesita ffmpeg real
   - No necesita red

3. **Rápido**
   - Se ejecuta en ~1 segundo
   - Usa repositorio en memoria
   - Archivos temporales pequeños

4. **Determinista**
   - Sin race conditions
   - Sin timeouts reales
   - Resultados consistentes

5. **Coverage Alto**
   - Cubre flujo completo
   - Todos los estados
   - Manejo de errores

## 🔍 Output Esperado

```
=== RUN   TestServerWorkerIntegration
=== RUN   TestServerWorkerIntegration/Schedule_Job
    ✅ Job scheduled: 123e4567-e89b-12d3-a456-426614174000
=== RUN   TestServerWorkerIntegration/Request_Job
    ✅ Job assigned to worker: 123e4567-e89b-12d3-a456-426614174000 (EventID: 2)
=== RUN   TestServerWorkerIntegration/Worker_Starts_Job
    ✅ Job started by worker
=== RUN   TestServerWorkerIntegration/Simulate_Encoding_Process
=== RUN   TestServerWorkerIntegration/Simulate_Encoding_Process/Download_Progress
    ✅ Download progress simulated
    ✅ Download completed
    ✅ Encoding completed
    ✅ Upload completed
=== RUN   TestServerWorkerIntegration/Complete_Job
    ✅ Job completed successfully!
       Source: test-video.mkv (1048576 bytes)
       Target: test-video_encoded.mkv (524288 bytes)
       Compression: 50.00%
=== RUN   TestServerWorkerIntegration/Verify_Event_History
    ✅ Event history verified (7 events)
       1. Job - queued
       2. Job - assigned
       3. Job - started
       4. Download - completed
       5. FFMPEG - completed
       6. Upload - completed
       7. Job - completed
--- PASS: TestServerWorkerIntegration (0.50s)
PASS
```

## 🚧 Futuras Mejoras

- [ ] Añadir test de job fallido
- [ ] Simular timeout de worker
- [ ] Test de cancelación de job
- [ ] Test de retry automático
- [ ] Test con múltiples workers
- [ ] Test de concurrencia
- [ ] Test de limpieza de archivos

## 📝 Notas

- Los tests usan build tag `integration` para separarse de unit tests
- Se ejecutan con `-short` para skippearse en runs rápidos
- Usan `t.TempDir()` para limpieza automática
- Repository mock es thread-safe con sync.RWMutex

