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

  getTerminalURL: (terminal) ->
    if container = App.get 'container'
      protocol = location.protocol.replace('http', 'ws')
      freighterdUrl = container.get 'freighterdUrl'
      cid = container.get 'cid'
      token = encodeURIComponent(container.get 'token')
      term = terminal.term

      url = "#{protocol}//#{freighterdUrl}/containers/#{cid}/shell?token=#{token}&width=#{term.cols}&height=#{term.rows}"
      command = encodeURIComponent(getLoginCommand(container.get 'loginUsername'))
      url += "&command=#{command}"

getLoginCommand = (username) ->
  username ||= "root"
  loginCmd = "/bin/login -f #{username} -p TERM=xterm-256color"
  "script -q -c '#{loginCmd}' /dev/null || #{loginCmd}"
