App.ContainerTerminalController = Em.ObjectController.extend
  freighterdUrlBinding: Em.Binding.oneWay 'model.host.freighterdUrl'
  ready: Em.computed.notEmpty 'freighterdUrl'

  start: (terminal) ->
    container = @get 'model'
    freighterdUrl = @get 'freighterdUrl'
    term = terminal.term

    container.requestToken(['shell']).then (token) =>
      token = encodeURIComponent(token.token)
      command = encodeURIComponent(getLoginCommand())

      protocol = location.protocol.replace('http', 'ws')
      url = "#{protocol}//#{freighterdUrl}/containers/#{container.get 'cid'}/shell?token=#{token}&command=#{command}&width=#{term.cols}&height=#{term.rows}"

      terminal.start(url)
    , (jqxhr) ->
      console.log jqxhr
      App.snackbar.post('Could not request shell!')

getLoginCommand = (username) ->
  username ||= "root"
  "/sbin/getty --noclear -i -a #{username} -8 38400 tty xterm-256color 2>/dev/null -o \"-f #{username} -p\" || /bin/login -f #{username} -p TERM=xterm-256color"
