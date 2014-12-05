#FIXME this file is only for testing purposes.
dummyFileTree = JSON.parse("[[\".bashrc\",false,531],[\".bash_profile\",false,116],[\".parts\",true,4096],[\"simple-todos\",true,4096],[\".gitconfig\",false,134],[\".meteorsession\",false,90],[\".cache\",true,4096],[\".nitrousboxrc.sample\",false,520],[\"workspace\",true,4096],[\".ssh\",true,4096],[\".tmux.conf\",false,88],[\".VERSION\",false,28],[\"README.md\",false,2026],[\".bash_history\",false,477],[\".viminfo\",false,981],[\".dbshell\",false,2535],[\".gemrc\",false,76],[\".pam_environment\",false,55],[\".meteor\",false,39]]")
dummyOutput = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Maecenas non eros sit amet nulla placerat facilisis eu dignissim neque. Vestibulum sodales nisl purus, at elementum leo efficitur sit amet."

App.DummySocket = Em.Object.extend
  name: 'sftp'
  started: false
  startedFirstTime: false
  startedAt: null

  connect: ->
    @set 'startedAt', Date.now()
    @set 'started', true
    @set('startedFirstTime', true) if !@get('startedFirstTime')

  disconnect: ->
    @setProperties
      startedFirstTime: false
      started: false
      startedAt: null

  fetchFileList: (absolutePath, callback) ->
    callback(null, absolutePath, dummyFileTree)

  fetchFileContent: (absolutePath, callback) ->
    callback(null, absolutePath, 'text/plain', dummyOutput)

  downloadFile: (absolutePath, callback) ->
    callback(null, '/tmp/file')

  writeFile: (absolutePath, content, callback) ->
    callback(null, absolutePath)

  writeNewFile: (absolutePath, content, callback) ->
    callback(null, absolutePath)

  makeDirectory: (absolutePath, callback) ->
    callback()

  sendUploadToken: (absolutePath, token, callback) ->
    callback(null, absolutePath)

  deleteFile: (absolutePath, callback) ->
    callback(null)

  deleteDir: (absolutePath, callback) ->
    callback(null)

  rename: (currentPath, newPath, callback) ->
    callback(null)
