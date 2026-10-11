# Transferencia de medios

## Purpose

Especifica cómo un worker asignado descarga el origen, comprueba su integridad y entrega el resultado al servidor.

## Requirements

### Requirement: Descarga autorizada por asignación
`GET /api/v1/download?uuid=<id>` SHALL servir el origen únicamente cuando el trabajo esté en estado `started` y la cabecera `workerName` coincida con el worker asignado.

#### Scenario: Worker asignado
- **WHEN** el worker asignado solicita un trabajo iniciado
- **THEN** la API devuelve HTTP 200 con el contenido, su longitud y el nombre del archivo

#### Scenario: Worker distinto
- **WHEN** otro worker solicita el mismo archivo
- **THEN** la API devuelve HTTP 403

#### Scenario: Archivo de origen ausente
- **WHEN** el origen de un trabajo autorizado ya no existe
- **THEN** la API devuelve HTTP 404

### Requirement: Comprobación de descarga
El servidor SHALL calcular SHA-256 durante la lectura del origen y exponer el valor mediante `GET /api/v1/checksum?uuid=<id>` una vez publicada la suma. El worker SHALL comparar la longitud y la suma obtenidas con las del servidor antes de continuar.

#### Scenario: Descarga íntegra
- **WHEN** coinciden la longitud recibida y el SHA-256 publicado
- **THEN** el worker analiza el medio descargado y continúa la cadena

#### Scenario: Suma distinta
- **WHEN** el SHA-256 local no coincide con el publicado
- **THEN** la fase de descarga falla

### Requirement: Subida del resultado
`POST` y `PUT /api/v1/upload?uuid=<id>` SHALL aceptar datos de un worker asignado a un trabajo `started` y comprobar longitud y SHA-256 declarados antes de responder HTTP 201.

#### Scenario: Resultado íntegro
- **WHEN** coinciden longitud y suma SHA-256 de la subida
- **THEN** la API devuelve HTTP 201

#### Scenario: Longitud o suma incorrecta
- **WHEN** la longitud o la suma SHA-256 no coincide
- **THEN** la API devuelve HTTP 400

#### Scenario: Worker no asignado
- **WHEN** un worker distinto intenta subir el resultado
- **THEN** la API devuelve HTTP 403

### Requirement: Comprobación del resultado por el worker
El worker SHALL calcular SHA-256 del archivo codificado y enviarlo con la subida. SHALL publicar `Job/completed` solo después de una respuesta HTTP 201.

#### Scenario: Subida aceptada
- **WHEN** el servidor responde HTTP 201
- **THEN** el worker publica la finalización del trabajo y limpia su directorio temporal

#### Scenario: Subida rechazada
- **WHEN** el servidor devuelve un estado distinto de HTTP 201
- **THEN** el worker considera fallida la tentativa y no publica `Job/completed` por ella
