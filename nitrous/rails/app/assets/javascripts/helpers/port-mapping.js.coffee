Em.Handlebars.helper 'port-mapping', (portMapping) ->
  { scope, hostPort, containerPort, protocol } = portMapping
  "#{scope}:#{hostPort}:#{containerPort}/#{protocol}"