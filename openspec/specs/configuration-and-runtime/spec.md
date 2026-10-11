# Configuración y ejecución

## Purpose

Define cómo arrancan el servidor y los workers, cómo reciben su configuración y qué servicios necesitan para prestar el servicio de transcodificación.

## Requirements

### Requirement: Configuración de los procesos
Cada binario SHALL aceptar opciones de línea de comandos y un archivo YAML de configuración, con valores predeterminados para las opciones registradas. SHALL permitir variables de entorno con el prefijo `TR_`.

#### Scenario: Arranque sin archivo YAML
- **WHEN** no existe un archivo de configuración en las rutas de búsqueda
- **THEN** el proceso continúa con los valores de las opciones registradas y registra un aviso

#### Scenario: Opciones explícitas
- **WHEN** se proporciona una opción de línea de comandos para una clave configurada
- **THEN** el proceso utiliza ese valor para la clave

### Requirement: Arranque del servidor
El servidor SHALL inicializar la base de datos antes de poner en marcha el planificador y la API HTTP. La ruta raíz de los archivos fuente SHALL ser absoluta.

#### Scenario: Base de datos no inicializable
- **WHEN** la inicialización de PostgreSQL falla
- **THEN** el servidor no inicia el planificador ni la API HTTP

#### Scenario: Servicios inicializados
- **WHEN** la configuración y la base de datos son válidas
- **THEN** el servidor inicia el planificador y escucha en el puerto HTTP configurado

### Requirement: Arranque del worker
El worker SHALL identificarse ante el servidor mediante un evento inicial antes de aceptar trabajos y SHALL crear un espacio temporal propio basado en su nombre.

#### Scenario: Servidor inaccesible al inicio
- **WHEN** falla el primer evento de presencia del worker
- **THEN** el worker no inicia la coordinación de trabajos

#### Scenario: Arranque correcto
- **WHEN** el primer evento de presencia se acepta
- **THEN** el worker prepara su espacio temporal, sus ejecutores y la coordinación con el servidor

### Requirement: Parada por señal
Ambos procesos SHALL responder a las señales de terminación deteniendo sus servicios asociados.

#### Scenario: Señal de terminación del servidor
- **WHEN** el servidor recibe una señal de terminación
- **THEN** cancela su contexto de ejecución y solicita el cierre de la API HTTP y del planificador

#### Scenario: Señal de terminación del worker
- **WHEN** el worker recibe una señal de terminación
- **THEN** cancela la coordinación y detiene los ejecutores y la consola
