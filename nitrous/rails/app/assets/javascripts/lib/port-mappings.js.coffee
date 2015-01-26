serializePortMapping = (portMapping) ->
  { scope, hostPort, containerPort } = portMapping
  "#{scope}:#{hostPort}:#{containerPort}"

parsePort = (port) ->
  parseInt(port, 10) || ''

Lib.PortMappings = Em.Object.extend
  parsedPorts: []

  init: () ->
    rawPorts = @getWithDefault 'ports', []

    parsedPorts = []
    @set 'parsedPorts', parsedPorts

    for rawPort in rawPorts
      [scope, hostPort, containerPort] = rawPort.split(':')
      parsedPorts.pushObject
        scope: scope
        hostPort: parsePort(hostPort)
        containerPort: parsePort(containerPort)

  add: (hostPort, containerPort, scope='private') ->
    hostPort = parsePort(hostPort)
    containerPort = parsePort(containerPort)

    parsedPorts = @get 'parsedPorts'
    
    for portMapping in parsedPorts
      if containerPort == portMapping.containerPort
        return
      if hostPort != '' && hostPort == portMapping.hostPort
        return

    parsedPorts.pushObject
      scope: scope
      hostPort: hostPort
      containerPort: containerPort

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
