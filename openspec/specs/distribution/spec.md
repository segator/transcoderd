# Distribución

## Purpose

Describe los ejecutables y contenedores que el proyecto entrega para ejecutar el servidor y el worker con sus dependencias de medios.

## Requirements

### Requirement: Binarios separados
La compilación SHALL producir ejecutables separados `transcoderd-server` y `transcoderd-worker`, con nombre, versión, commit y fecha de compilación incorporados.

#### Scenario: Construcción de Go
- **WHEN** se ejecuta la construcción de ambos binarios
- **THEN** aparecen `dist/transcoderd-server` y `dist/transcoderd-worker`

#### Scenario: Consulta de versión
- **WHEN** se ejecuta un binario con `--version`
- **THEN** muestra su información de versión y termina

### Requirement: Imágenes de servidor y worker
La distribución en contenedores SHALL proporcionar objetivos separados `server` y `worker`. El worker SHALL incluir las herramientas de análisis, codificación, extracción y OCR que usa su cadena de ejecución.

#### Scenario: Imagen de servidor
- **WHEN** se construye el objetivo `server`
- **THEN** su punto de entrada ejecuta `transcoderd-server`

#### Scenario: Imagen de worker
- **WHEN** se construye el objetivo `worker`
- **THEN** su punto de entrada ejecuta `transcoderd-worker` con las herramientas de medios disponibles

### Requirement: Etiquetas de distribución
Los objetivos de publicación SHALL etiquetar las imágenes con el nombre de aplicación, el rol `server` o `worker` y la versión solicitada.

#### Scenario: Publicación versionada
- **WHEN** se publica una versión de ambas imágenes
- **THEN** existen etiquetas `ghcr.io/segator/transcoderd:server-<version>` y `ghcr.io/segator/transcoderd:worker-<version>`
