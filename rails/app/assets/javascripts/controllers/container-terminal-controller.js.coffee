App.ContainerTerminalController = Em.ObjectController.extend
  queryParams: ['action', 'fullscreen']
  action: null
  fullscreen: false

  freighterdUrlBinding: Em.Binding.oneWay 'model.host.freighterdUrl'
  ready: Em.computed.notEmpty 'freighterdUrl'

  start: (terminal) ->
    container = @get 'model'
    freighterdUrl = @get 'freighterdUrl'
    term = terminal.term

    action = @get('action') || 'shell'
    if ['shell', 'attach', 'logs'].indexOf(action) == -1
      action = 'shell'

    container.requestToken([action]).then (token) =>
      token = encodeURIComponent(token.token)

      protocol = location.protocol.replace('http', 'ws')
      url = "#{protocol}//#{freighterdUrl}/containers/#{container.get 'cid'}/#{action}?token=#{token}&width=#{term.cols}&height=#{term.rows}"

      readonly = false

      switch action
        when 'shell'
          command = encodeURIComponent(getLoginCommand())
          url += "&command=#{command}"
        when 'logs'
          url += "&follow=true"
          readonly = true
          term.convertEol = true
        when 'attach'
          term.convertEol = true

      terminal.start(url, readonly)
    , (jqxhr) ->
      App.postError(jqxhr.responseJSON, 'Could not connect to shell!')

getLoginCommand = (username) ->
  username ||= "root"
  "/sbin/getty --noclear -i -a #{username} -8 38400 tty xterm-256color 2>/dev/null -o \"-f #{username} -p\" || /bin/login -f #{username} -p TERM=xterm-256color"
