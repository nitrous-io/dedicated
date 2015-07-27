rawPortRegex = /^(public|private):(\d{0,5}):(\d{1,5})(?:\/(tcp|udp))?$/

parsePort = (port) ->
  parseInt(port, 10) || ''

createPortMapping = (scope, hostPort, containerPort, protocol='tcp') ->
  return {
    scope: scope
    hostPort: parsePort(hostPort)
    containerPort: parsePort(containerPort)
    protocol: protocol
  }

parsePortMapping = (rawPortMapping) ->
  matches = rawPortRegex.exec(rawPortMapping)

  if !matches
    return null

  [_, scope, hostPort, containerPort, protocol] = matches

  return createPortMapping(scope, hostPort, containerPort, protocol)

# Two port mappings are conflicting if they have either the
# same container or host port and in the same protocol.
conflictPortMapping = (pm1, pm2) ->
  return (pm1.protocol == pm2.protocol) && 
    ((pm1.containerPort == pm2.containerPort) ||
     (pm1.hostPort != '' && pm1.hostPort == pm2.hostPort))

serializePortMapping = (portMapping) ->
  { scope, hostPort, containerPort, protocol } = portMapping
  "#{scope}:#{hostPort}:#{containerPort}/#{protocol}"

Lib.PortMappings = Em.Object.extend
  parsedPorts: []

  init: () ->
    rawPorts = @getWithDefault 'ports', []

    parsedPorts = []
    @set 'parsedPorts', parsedPorts

    for rawPort in rawPorts
      parsedPort = parsePortMapping(rawPort)

      if parsedPort
        parsedPorts.pushObject parsedPort

  add: (scope, hostPort, containerPort, protocol='tcp') ->
    hostPort = parsePort(hostPort)
    containerPort = parsePort(containerPort)

    parsedPorts = @get('parsedPorts')
    newPortMapping = createPortMapping(scope, hostPort, containerPort, protocol)
    
    for portMapping in parsedPorts
      if conflictPortMapping(newPortMapping, portMapping)
        # skip if both port mappings are the same
        # ignore if either host or container port already inside the current list
        return

    parsedPorts.pushObject(newPortMapping)

  remove: (portMapping) ->
    parsedPorts = @get 'parsedPorts'
    index = parsedPorts.map(serializePortMapping)
      .indexOf(portMapping)

    if index != -1
      parsedPorts.removeAt(index)

  rawPorts: (->
    parsedPorts = @get 'parsedPorts'
    return parsedPorts.sortBy('scope:desc', 'containerPort')
      .map(serializePortMapping)
      
  ).property 'parsedPorts.@each'
