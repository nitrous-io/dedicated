App.SearchCommand = Em.Object.extend
  idDataDelimiter: "!!\n"

  resultDocuments: null

  init: ->
    @set('resultDocuments', Em.Object.create())

  bindEvents: (socket) ->
    socket.on 'search.commandOutput', (data) =>
      this.onOutput(data)
    socket.on 'search.commandError', (data) =>
      this.onError(data)
    socket.on 'search.commandComplete', (data) =>
      this.onComplete(data)

  send: (socket, term, path, options, callback) ->
    tabId = (Date.now() + parseInt(Math.random() * 1000)).toString()
    socket.sendAs 'search', 'search', tabId, term, path, options, =>
      callback(this, tabId)

  onOutput: (data) ->
    [tabId, results] = this.parseIdData(data)
    if doc = @get("resultDocuments.#{tabId}")
      Em.run.next ->
        doc.addSearchResults(results)

  onError: (data) ->
    [tabId, error] = this.parseIdData(data)
    if doc = @get("resultDocuments.#{tabId}")
      Em.run.next ->
        doc.addError(error)

  onComplete: (data) ->
    [tabId, code] = this.parseIdData(data)
    if doc = @get("resultDocuments.#{tabId}")
      Em.run.next ->
        doc.addCurrentResult()
        doc.doComplete()

  parseIdData: (data) ->
    data.split(this.idDataDelimiter)
