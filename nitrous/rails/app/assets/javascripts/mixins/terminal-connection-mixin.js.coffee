App.TerminalConnectionMixin = Ember.Mixin.create
  makeUrl: (container, terminal, action) ->
    unless container
      return Ember.RSVP.Promise.reject(new Error('container not found'))

    term = terminal.term
    cid = container.get 'cid'
    freighterdUrl = container.get 'freighterdUrl'

    if ['shell', 'attach', 'logs'].indexOf(action) == -1
      action = 'shell'

    container.requestToken([action]).then (token) ->
      token = encodeURIComponent(token.token)

      protocol = location.protocol.replace('http', 'ws')
      url = "#{protocol}//#{freighterdUrl}/containers/#{cid}/#{action}?token=#{token}&width=#{term.cols}&height=#{term.rows}"

      switch action
        when 'shell'
          command = encodeURIComponent(getLoginCommand(container.get 'loginUsername'))
          url += "&command=#{command}"
        when 'logs'
          url += "&follow=true"
          terminal.readonly = true
          term.convertEol = true
        when 'attach'
          term.convertEol = true
      url

  connectTerminal: (container, terminal, action) ->
    container.getWebSocketFeatures().then ->
      console.log('websocketFeatures:', websocketFeatures)

    @makeUrl(container, terminal, action).then (url) ->
      terminal.url = url
      terminal.open(true)
      terminal.focus()
    , (jqxhr) ->
      terminal.retry()
      App.postError(jqxhr.responseJSON, 'Could not connect to shell, retrying...')

getLoginCommand = (username) ->
  username ||= "root"
  loginCmd = "/bin/login -f #{username} -p TERM=xterm-256color"
  "script -q -c '#{loginCmd}' /dev/null || #{loginCmd}"
