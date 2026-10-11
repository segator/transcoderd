# Persistencia y estado

## Purpose

Establece los datos duraderos que necesita el servidor para conservar trabajos, eventos y presencia de workers, y los datos temporales de progreso.

## Requirements

### Requirement: Evolución de la base de datos
El servidor SHALL aplicar, en orden de versión, las migraciones SQL pendientes al arrancar y SHALL registrar cada versión aplicada.

#### Scenario: Base de datos nueva
- **WHEN** no hay versiones registradas
- **THEN** se crea el registro de versiones y se aplican las migraciones disponibles

#### Scenario: Versión ya aplicada
- **WHEN** una migración ya figura en el registro de versiones
- **THEN** no se vuelve a ejecutar al siguiente arranque

### Requirement: Registro duradero del trabajo
El servidor SHALL guardar para cada trabajo su identificador, rutas de origen y destino y tamaños, y SHALL mantener su historial de eventos asociado.

#### Scenario: Trabajo nuevo
- **WHEN** se incorpora un archivo elegible que no existe en la base de datos
- **THEN** se guarda el trabajo y su primer evento `Job/queued`

#### Scenario: Consulta interna por identificador
- **WHEN** el servidor consulta un trabajo existente por UUID
- **THEN** recupera sus datos, historial y estado más reciente

### Requirement: Estado proyectado
El servidor SHALL actualizar el estado actual de cada trabajo a partir de las notificaciones insertadas para permitir seleccionar trabajos por fase y estado.

#### Scenario: Asignación registrada
- **WHEN** se inserta un evento `Job/assigned`
- **THEN** el estado proyectado del trabajo refleja la asignación

#### Scenario: Finalización registrada
- **WHEN** se inserta un evento `Job/completed`
- **THEN** el estado proyectado refleja la finalización

### Requirement: Presencia y progreso de workers
El servidor SHALL actualizar el último contacto y dirección de un worker por su nombre, y SHALL conservar el progreso activo de cada fase hasta recibir un estado final o limpiarlo por antigüedad.

#### Scenario: Presencia repetida
- **WHEN** un worker existente envía otro `Ping`
- **THEN** se actualizan su dirección y fecha de último contacto

#### Scenario: Progreso de una fase
- **WHEN** se recibe un progreso activo
- **THEN** quedan disponibles su porcentaje, ETA, worker y fecha de actualización en el almacenamiento de progreso
