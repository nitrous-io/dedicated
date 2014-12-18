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

  openTerminal: (terminal) ->
    if container = App.get 'container'
      protocol = location.protocol.replace('http', 'ws')
      freighterdUrl = container.get 'freighterdUrl'
      cid = container.get 'cid'
      term = terminal.term

      container.requestToken(["shell"]).then (token) =>
        token = encodeURIComponent(token.token)

        url = "#{protocol}//#{freighterdUrl}/containers/#{cid}/shell?token=#{token}&width=#{term.cols}&height=#{term.rows}"
        command = encodeURIComponent(getLoginCommand(container.get 'loginUsername'))
        url += "&command=#{command}"

        # continue to load the terminal
        terminal.url = url
        terminal.open(true)
        terminal.focus()
      , (jqxhr) ->
        terminal.retry()

getLoginCommand = (username) ->
  username ||= "root"
  loginCmd = "/bin/login -f #{username} -p TERM=xterm-256color"
  "script -q -c '#{loginCmd}' /dev/null || #{loginCmd}"
