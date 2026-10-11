# Eventos y progreso

## Purpose

Define el intercambio de eventos entre workers y servidor, el historial del ciclo de vida de los trabajos y los datos temporales de progreso.

## Requirements

### Requirement: Publicación de eventos
La API SHALL recibir `POST /api/v1/event` con un sobre `eventType` y `eventData`, y SHALL reconocer eventos `Ping`, `Notification` y `Progress`.

#### Scenario: Presencia del worker
- **WHEN** se recibe un evento `Ping` válido
- **THEN** se registra o actualiza la presencia del worker y la API responde HTTP 200

#### Scenario: Tipo desconocido
- **WHEN** se recibe un tipo de evento distinto de los admitidos
- **THEN** el servidor rechaza el evento con un error

### Requirement: Historial secuencial de notificaciones
Cada notificación de un trabajo SHALL conservar su tipo de fase, estado, worker, mensaje y número de evento. El servidor SHALL aceptar un número de evento únicamente cuando siga al último número guardado para ese trabajo.

#### Scenario: Siguiente notificación
- **WHEN** llega una notificación con `eventID` inmediatamente posterior al último del trabajo
- **THEN** el servidor la guarda y actualiza el estado proyectado del trabajo

#### Scenario: Número fuera de secuencia
- **WHEN** llega una notificación con un `eventID` repetido o saltado
- **THEN** el servidor devuelve un error y no la añade al historial

### Requirement: Finalización del trabajo
Al recibir una notificación `Job/completed`, el servidor SHALL leer el archivo de destino, actualizar su tamaño persistido y, si está configurado, eliminar el archivo de origen.

#### Scenario: Archivo de destino presente
- **WHEN** el worker notifica `Job/completed` y el destino existe
- **THEN** el tamaño del destino queda registrado en el trabajo

#### Scenario: Eliminación habilitada
- **WHEN** `deleteOnComplete` está activado y el destino existe
- **THEN** el servidor intenta eliminar el archivo de origen tras registrar el tamaño del destino

### Requirement: Progreso temporal
El servidor SHALL guardar los eventos `Progress` con estado `progressing` por identificador de progreso y tipo de fase, y SHALL retirar esa entrada al recibir un estado final.

#### Scenario: Progreso en curso
- **WHEN** llega `Progress/progressing` con porcentaje y ETA
- **THEN** la entrada de progreso se inserta o actualiza

#### Scenario: Progreso terminado
- **WHEN** llega `Progress/done` o `Progress/failure`
- **THEN** la entrada de progreso correspondiente se elimina

### Requirement: Notificación de fallos de fase
El worker SHALL publicar el fallo de la fase y un fallo de trabajo cuando una acción de la cadena falla, y SHALL limpiar su espacio temporal del trabajo.

#### Scenario: Error de codificación
- **WHEN** falla la acción de codificación
- **THEN** el worker publica `FFMPEG/failed`, publica `Job/failed` con el error y limpia el contexto temporal
