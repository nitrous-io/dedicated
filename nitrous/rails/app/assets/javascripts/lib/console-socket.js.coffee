#= require ./socket/socket

#App.ConsoleSocket = Lib.Socket.extend
  #container: null

  #connect: ->
    #return unless @get 'container'
    #@_super.apply this, arguments

  #disconnect: ->
    #@_super.apply this, arguments
    #@set 'container', null

  #willOpen: (ev) ->
    #@_super.apply this, arguments
    #container = @get 'container'
    #protocol = location.protocol.replace('http', 'ws')
    #freighterdUrl = container.get 'freighterdUrl'
    #cid = container.get 'cid'

    #container.requestToken(['ops']).then (token) =>
      #token = encodeURIComponent(token.token)

      #url = "#{protocol}//#{freighterdUrl}/containers/#{cid}/shell?token=#{token}&width=#{term.cols}&height=#{term.rows}"
      #command = encodeURIComponent(getLoginCommand(container.get 'loginUsername'))
      #url += "&command=#{command}"

      #@set 'url', url

      ## proceed with connection
      #@connect(true)
    #, (jqxhr) ->
      #@retry()

#getLoginCommand = (username) ->
  #username ||= "root"
  #loginCmd = "/bin/login -f #{username} -p TERM=xterm-256color"
  #"script -q -c '#{loginCmd}' /dev/null || #{loginCmd}"
