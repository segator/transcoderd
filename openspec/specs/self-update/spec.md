# Actualización de binarios

## Purpose

Documenta la comprobación de versiones publicadas y la sustitución automática de los binarios del servidor y del worker cuando está habilitada.

## Requirements

### Requirement: Comparación de versiones
Cada proceso SHALL consultar la versión de la última publicación del proyecto y SHALL compararla semánticamente con su versión actual según el intervalo configurado.

#### Scenario: Versión más reciente
- **WHEN** la publicación más reciente tiene una versión superior a la ejecutada
- **THEN** el proceso identifica que hay una actualización disponible

#### Scenario: Comprobación dentro del intervalo
- **WHEN** aún no ha transcurrido `updateCheckInterval`
- **THEN** se omite una nueva consulta remota

### Requirement: Modo sin actualizaciones
La opción `noUpdates` SHALL impedir la sustitución del binario aunque exista una versión superior.

#### Scenario: Actualizaciones deshabilitadas
- **WHEN** `noUpdates` está activado y existe una publicación más reciente
- **THEN** el proceso registra el aviso y continúa sin aplicar esa publicación

### Requirement: Supervisión y sustitución
Con actualizaciones habilitadas, el proceso supervisor SHALL obtener el recurso de la publicación cuyo nombre corresponda al binario y SHALL sustituir el ejecutable antes de arrancar de nuevo la aplicación.

#### Scenario: Recurso coincidente
- **WHEN** una versión superior incluye el recurso del servidor o worker correspondiente
- **THEN** el supervisor descarga ese recurso y aplica la actualización del ejecutable

#### Scenario: Recurso ausente
- **WHEN** la publicación no contiene el recurso correspondiente
- **THEN** la actualización devuelve un error y no arranca con ese recurso

### Requirement: Comprobaciones durante el servicio
El worker SHALL comprobar actualizaciones antes de solicitar un nuevo trabajo. El servidor SHALL comprobarlas periódicamente solo cuando no haya peticiones HTTP activas.

#### Scenario: Servidor ocupado
- **WHEN** vence la comprobación periódica y hay una petición HTTP activa
- **THEN** el servidor pospone la consulta de actualización

#### Scenario: Worker disponible
- **WHEN** el worker admite trabajo y alcanza su ciclo de consulta
- **THEN** comprueba si necesita actualizarse antes de pedir una asignación
