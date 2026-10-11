# Asignación de trabajos

## Purpose

Establece el contrato de solicitud de trabajo, asignación a un worker y control de la admisión de nuevas tareas en el worker.

## Requirements

### Requirement: Solicitud del siguiente trabajo
La API SHALL servir `GET /api/v1/job/request` a workers identificados con la cabecera `workerName`. Un trabajo disponible SHALL producir un identificador y el último `eventID` de asignación.

#### Scenario: Trabajo en cola
- **WHEN** un worker solicita trabajo y existe un trabajo `queued`
- **THEN** el servidor añade un evento `Job/assigned` con el nombre del worker y responde HTTP 200 con `id` y `eventID`

#### Scenario: Cola vacía
- **WHEN** no existe ningún trabajo `queued`
- **THEN** la API responde HTTP 204

#### Scenario: Nombre de worker ausente
- **WHEN** falta la cabecera `workerName`
- **THEN** la API responde HTTP 403

### Requirement: Coordinación periódica del worker
El worker SHALL publicar presencia y consultar trabajos periódicamente mientras esté activo.

#### Scenario: Worker activo
- **WHEN** transcurre el intervalo de coordinación de cinco segundos
- **THEN** el worker envía un evento de presencia e intenta solicitar trabajo si acepta nuevas tareas

### Requirement: Admisión local de trabajos
El worker SHALL rechazar nuevos trabajos mientras esté pausado; sin una ventana horaria configurada SHALL limitar a dos sus trabajos activos. Con una ventana configurada SHALL aceptar solicitudes solo dentro del intervalo local indicado.

#### Scenario: Worker pausado
- **WHEN** el worker está pausado
- **THEN** no solicita un nuevo trabajo

#### Scenario: Límite ordinario alcanzado
- **WHEN** no hay ventana horaria configurada y el worker ya tiene dos trabajos activos
- **THEN** no solicita otro trabajo

#### Scenario: Fuera de la ventana
- **WHEN** el reloj local queda fuera de `startAfter` y `stopAfter`
- **THEN** el worker no solicita trabajo

### Requirement: Inicio de una asignación
El worker SHALL preparar un contexto temporal para el trabajo asignado y publicar `Job/started` antes de ejecutar la descarga.

#### Scenario: Asignación recibida
- **WHEN** el worker recibe `id` y `eventID` de un trabajo asignado
- **THEN** crea el directorio del trabajo, publica `Job/started` y encola la descarga
