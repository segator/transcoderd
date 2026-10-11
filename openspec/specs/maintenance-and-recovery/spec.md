# Mantenimiento y recuperación

## Purpose

Describe cómo el servidor revisa trabajos atascados o fallidos y cómo el worker retoma un contexto temporal que quedó tras un reinicio.

## Requirements

### Requirement: Revisión periódica de trabajos
El servidor SHALL ejecutar mantenimiento de trabajos según `scheduleTime` y SHALL revisar los trabajos en cola, fallidos y asignados.

#### Scenario: Origen en cola desaparecido
- **WHEN** el origen de un trabajo `queued` ya no existe durante el mantenimiento
- **THEN** se añade un evento `Job/failed` que indica que falta el archivo

### Requirement: Recuperación por tiempo de espera
El servidor SHALL volver a encolar un trabajo `assigned` o `started` cuyo último evento de trabajo exceda `jobTimeout`, registrando antes una cancelación de esa asignación.

#### Scenario: Worker sin terminar el trabajo
- **WHEN** el último evento de un trabajo asignado o iniciado supera `jobTimeout`
- **THEN** se añaden eventos `Job/canceled` y `Job/queued` para permitir otra asignación

### Requirement: Reintento de fallos clasificados
El servidor SHALL volver a encolar fallos clasificados como recuperables durante el mantenimiento y SHALL cancelar el trabajo tras más de diez eventos `Job/failed` de ese historial.

#### Scenario: Fallo recuperable
- **WHEN** un trabajo `failed` tiene un mensaje clasificado como recuperable y no supera el umbral de intentos
- **THEN** se añade un evento `Job/queued`

#### Scenario: Demasiados fallos
- **WHEN** un trabajo recuperable acumula más de diez eventos `Job/failed`
- **THEN** se añade un evento `Job/canceled` por exceso de intentos

#### Scenario: Fallo no clasificado
- **WHEN** el mensaje del fallo no está clasificado como recuperable
- **THEN** el mantenimiento no lo reencola automáticamente

### Requirement: Limpieza del progreso obsoleto
El servidor SHALL eliminar las entradas de progreso cuya última actualización tenga más de 24 horas.

#### Scenario: Progreso abandonado
- **WHEN** una entrada de progreso supera 24 horas sin actualización
- **THEN** desaparece durante la siguiente revisión de progreso

### Requirement: Recuperación de contextos locales
Al iniciar, el worker SHALL buscar contextos JSON en su espacio temporal y SHALL reanudar cada trabajo desde la fase indicada por su último evento guardado.

#### Scenario: Contexto persistido
- **WHEN** hay un contexto JSON válido de un trabajo anterior
- **THEN** el worker lo incorpora a sus trabajos activos y lo encola en la fase guardada
