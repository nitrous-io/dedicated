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

# Two port mappings are fuzzy equal if they have the 
# same container port, either their host ports are either empty
# or both host ports are the same.
#
# Used for matching port mappings with optional host ports.
# i.e. public:49153:1337/tcp == public::1337/tcp
#
# Main difference with conflictPortMapping is that there is
# conflict with different container ports but with the same host port
fuzzyEqualPortMapping = (pm1, pm2) ->
  return (pm1.protocol == pm2.protocol) && 
    ((pm1.containerPort == pm2.containerPort) &&
     ((pm1.hostPort == '' || pm2.hostPort == '') ||
      (pm1.hostPort == pm2.hostPort)))

serializePortMapping = (portMapping) ->
  { scope, hostPort, containerPort, protocol } = portMapping
  "#{scope}:#{hostPort}:#{containerPort}/#{protocol}"

Lib.PortMappings = Em.Object.extend
  parsedPorts: []

  init: () ->
    rawPorts = @getWithDefault 'ports', []

    parsedPorts = rawPorts.map(parsePortMapping)

    @set 'parsedPorts', parsedPorts

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
    @get('parsedPorts').removeObject(portMapping)

  fuzzyRemove: (portMapping) ->
    parsedPorts = @get 'parsedPorts'
    for parsedPort, i in parsedPorts
      if fuzzyEqualPortMapping(portMapping, parsedPort)
        return parsedPorts.removeAt(i)

  sortedPorts: (->
     @get('parsedPorts').sortBy('scope:desc', 'containerPort')
  ).property 'parsedPorts.@each'

