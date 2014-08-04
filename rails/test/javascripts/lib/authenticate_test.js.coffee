describe 'App.authenticate', ->
  server = cb = null

  beforeEach ->
    server = sinon.fakeServer.create()
    cb = sinon.spy()

  afterEach ->
    server.restore()

  doAction = ->
    App.authenticate "pete", "123456", cb

  context 'when server responds with 200 OK', ->
    it 'invokes callback with a Token object', ->
      server.respondWith "POST", "/api/v0/oauth/token", [
        200
        { 'Content-Type': 'application/json; charset=utf-8' }
        '{"access_token":"Abcde12345", "token_type": "Bearer"}'
      ]
      doAction()
      server.respond()
      expect(cb.calledOnce).to.be.true 'callback is invoked'
      args = cb.args[0]
      expect(args[0]).to.be.null 'no error is passed'
      expect(args[1]).to.equal 'Abcde12345', 'Token object has correct access token string'

  context 'when server responds with 400 Bad Request', ->
    it 'invokes callback with error', ->
      server.respondWith "POST", "/api/v0/oauth/token", [
        400
        { 'Content-Type': 'application/json; charset=utf-8' }
        '{"error":"invalid_grant", "error_description": "incorrect username or password"}'
      ]
      doAction()
      server.respond()
      expect(cb.calledOnce).to.be.true 'callback is invoked'
      args = cb.args[0]
      expect(args[0]).to.be.instanceof Error, 'error is passed'
      expect(args[0].type).to.equal 'invalid_grant', 'err.type is set correctly'
      expect(args[0].message).to.equal 'incorrect username or password', 'err.message is set correctly'
      expect(args[1]).not.to.be.defined 'no Token object is passed'
