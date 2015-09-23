rawPortRegex = /^(public|private):(\d{0,5}):(\d{1,5})(?:\/(tcp|udp))?$/

parsePort = (port) ->
  parseInt(port, 10) || ''

parsePortMapping = (rawPortMapping) ->
  matches = rawPortRegex.exec(rawPortMapping)
  if !matches
    return null

  [_, scope, hostPort, containerPort, protocol] = matches

  Lib.PortMapping.create
    scope: scope
    hostPort: parsePort(hostPort)
    containerPort: parsePort(containerPort)
    protocol: protocol

# Two port mappings are conflicting if they have either the
# same container or host port and in the same protocol.
conflictPortMapping = (pm1, pm2) ->
  return (pm1.protocol == pm2.protocol) &&
    ((pm1.containerPort == pm2.containerPort) ||
     (pm1.hostPort != '' && pm1.hostPort == pm2.hostPort))

Lib.PortMapping = Em.Object.extend
  scope: null
  hostPort: null # This is the configured host port. If null, it means it will be dynamically assigned.
  assignedHostPort: null
  containerPort: null
  protocol: 'tcp'

  public: (->
    @get('scope') == 'public'
  ).property 'scope'

  isHttpPort: (->
    port = @get('hostPort') || @get('assignedPort')
    port == 80
  ).property 'hostPort', 'assignedPort'

  isHttpsPort: (->
    port = @get('hostPort') || @get('assignedPort')
    port == 443
  ).property 'hostPort', 'assignedPort'

  # Returns true if this port mapping is the assigned version of the given port
  # mapping.
  isAssignedVersionOf: (mapping) ->
    @get('hostPort') && !mapping.get('hostPort') &&
      @get('scope') == mapping.get('scope') &&
      @get('containerPort') == mapping.get('containerPort') &&
      @get('protocol') == mapping.get('protocol')

Lib.PortMappings = Em.Object.extend
  content: Em.A()
  configuredPorts: Em.A() # port mappings of the form "public::22/tcp", "private:80:3000/tcp"
  livePorts: Em.A() # port mappings of the form "public:32768:22/tcp", "private:80:3000/tcp"

  init: ->
    livePorts = @get('livePorts')
    mappings = []
    for port in livePorts
      mapping = parsePortMapping(port)
      continue if !mapping
      mappings.pushObject(mapping)

    configuredPorts = @get('configuredPorts')
    for port in configuredPorts
      configuredMapping = parsePortMapping(port)
      if !configuredMapping || configuredMapping.get('hostPort')
        # Skip if this is not a dynamic port mapping.
        continue

      # Lookup the "live" port mapping of this dynamic port mapping.
      liveMapping = mappings.find((mapping) ->
        mapping.isAssignedVersionOf(configuredMapping)
      )
      continue if !liveMapping
      liveMapping.set('assignedHostPort', liveMapping.get('hostPort'))
      liveMapping.set('hostPort', '')

    @set 'content', Em.ArrayProxy.createWithMixins Em.SortableMixin,
      content: Em.A(mappings)
      sortProperties: ['containerPort']

  add: (scope, hostPort, containerPort, protocol='tcp') ->
    newPortMapping = Lib.PortMapping.create
      scope: scope
      hostPort: parsePort(hostPort)
      containerPort: parsePort(containerPort)
      protocol: protocol

    mappings = @get('content')
    return false if mappings.any((mapping) ->
      conflictPortMapping(newPortMapping, mapping)
    )

    mappings.pushObject(newPortMapping)
    true

  remove: (portMapping) ->
    mappings = @get('content')
    mappings = mappings.reject((m) ->
      portMapping == m
    )
    @set('content', mappings)

  # Converts port mappings so that they are usable as HTTP form params.
  serialized: (->
    @get('content').map((mapping) ->
      "#{mapping.scope}:#{mapping.hostPort}:#{mapping.containerPort}/#{mapping.protocol}"
    )
  ).property 'content.@each'
