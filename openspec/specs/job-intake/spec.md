# Incorporación de trabajos

## Purpose

Describe cómo una petición sobre un archivo o un directorio de la biblioteca se convierte en trabajos persistidos, y cuándo se permite volver a encolarlos.

## Requirements

### Requirement: Solicitud de incorporación
La API SHALL aceptar `POST /api/v1/job/` con JSON que contenga `sourcePath` y SHALL devolver las colecciones `scheduled`, `failed` y `skipped` de la operación.

#### Scenario: Ruta ausente
- **WHEN** la petición omite `sourcePath` o lo deja vacío
- **THEN** la API devuelve HTTP 400

#### Scenario: Solicitud procesada
- **WHEN** se presenta una ruta válida para el planificador
- **THEN** la API devuelve HTTP 200 con los resultados por archivo

### Requirement: Descubrimiento de archivos
El planificador SHALL recorrer la ruta solicitada bajo la raíz configurada y SHALL evaluar los archivos por tamaño mínimo y extensión admitida.

#### Scenario: Directorio con medios válidos
- **WHEN** la ruta solicitada contiene archivos con extensión admitida y tamaño suficiente
- **THEN** el planificador intenta crear un trabajo para cada archivo elegible

#### Scenario: Archivo no elegible
- **WHEN** un archivo está por debajo del tamaño mínimo o su extensión no está admitida
- **THEN** la respuesta incluye el archivo y el motivo en `failed`

### Requirement: Identidad y destino del trabajo
Cada trabajo nuevo SHALL recibir un identificador UUID, guardar rutas relativas de origen y destino, guardar el tamaño de origen y comenzar en estado `queued`. El destino SHALL usar la extensión `.mkv` y evitar sobrescribir un origen cuyo nombre de destino sería idéntico.

#### Scenario: Origen con otro contenedor
- **WHEN** un archivo elegible termina en `.avi`
- **THEN** la ruta de destino generada termina en `.mkv`

#### Scenario: Nombre de destino idéntico al origen
- **WHEN** la conversión del nombre deja la misma ruta que el origen
- **THEN** el destino recibe el sufijo `_encoded.mkv`

### Requirement: Reincorporación de trabajos existentes
Un trabajo existente SHALL volver a `queued` solo cuando la petición incluya el indicador de fuerza correspondiente a su estado `completed`, `failed` o `canceled`, o cuando una operación interna fuerce la recuperación de un trabajo `assigned` o `started`.

#### Scenario: Trabajo completado con fuerza
- **WHEN** se solicita un origen completado con `forceCompleted: true`
- **THEN** se añade un nuevo evento `Job/queued` al mismo trabajo

#### Scenario: Trabajo existente sin fuerza aplicable
- **WHEN** se solicita de nuevo un origen ya registrado sin un indicador compatible con su estado
- **THEN** el planificador informa de la imposibilidad de reencolarlo en `failed`

### Requirement: Cancelación de trabajos asignados
La API SHALL aceptar `DELETE /api/v1/job/{jobid}` y el servidor SHALL registrar `Job/canceled` para un trabajo `assigned` o `started` identificado por ese UUID.

#### Scenario: Trabajo iniciado
- **WHEN** se solicita cancelar un trabajo en estado `started`
- **THEN** se añade un evento `Job/canceled` con motivo de cancelación por usuario

#### Scenario: Trabajo completado
- **WHEN** se solicita cancelar un trabajo ya `completed`
- **THEN** el servidor devuelve un error y no añade un nuevo evento de cancelación
