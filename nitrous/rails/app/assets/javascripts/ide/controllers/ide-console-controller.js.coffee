App.IdeConsoleController = Em.ArrayController.extend App.IdeTabControllerMixin,
  needs: ['ideFileFinder']

  modelClass: App.ConsoleSession
  content: null
  fullscreen: false

  initialize: ->
    @set 'content', []
    @set 'fullscreen', (App.localStorage.get('fullscreenConsole') ? false)

  fullscreenDidChange: (->
    App.localStorage.set 'fullscreenConsole', @get('fullscreen')
  ).observes 'fullscreen'

  createTab: (attrs={}) ->
    App.util.mixpanelIncrement 'ide_new_console_tab_count'
    @_super(attrs)

  reset: ->
    @set 'content', null

  makeUrlAndConnect: (terminal) ->
    container = App.get 'container'
    freighterdUrl = container.get 'freighterdUrl'
    term = terminal.term

    container.requestToken(['shell']).then (token) =>
      token = encodeURIComponent(token.token)

      protocol = location.protocol.replace('http', 'ws')
      url = "#{protocol}//#{freighterdUrl}/containers/#{container.get 'cid'}/shell?token=#{token}&width=#{term.cols}&height=#{term.rows}"

      command = encodeURIComponent(getLoginCommand(container.get 'loginUsername'))
      url += "&command=#{command}"

      terminal.url = url
      # url obtained, resume connecting
      terminal.open(true)
    , (jqxhr) ->
      terminal.retry()
      # TODO post retrying message

getLoginCommand = (username) ->
  username ||= "root"
  loginCmd = "/bin/login -f #{username} -p TERM=xterm-256color"
  "script -q -c '#{loginCmd}' /dev/null || #{loginCmd}"
