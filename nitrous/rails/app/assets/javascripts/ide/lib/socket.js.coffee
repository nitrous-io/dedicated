#= require ../vendor/eventemitter2

window.App ||= {}

class App.Socket extends EventEmitter2
  constructor: (@name, @socketConnection) ->
    @emitter = new EventEmitter2
    @socketConnection.on @name, (action, args) =>
      this.emit action, args...

  destroy: ->
    @socketConnection.removeAllListeners()
    this.removeAllListeners()

  send: (event, args...) ->
    @socketConnection.send @name, event, args

  sendAs: (name, event, args...) ->
    @socketConnection.send name, event, args
