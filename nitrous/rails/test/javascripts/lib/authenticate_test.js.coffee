describe 'App.authenticate', ->
  server = cb = null

  beforeEach ->
    server = sinon.fakeServer.create()
    cb = sinon.spy()

  afterEach ->
    server.restore()

  context 'when server responds with 200 OK', ->
    it 'invokes callback with a Token object', ->
      server.respondWith "POST", "/api/v0/oauth/token", [
        200
        { 'Content-Type': 'application/json; charset=utf-8' }
        '{"access_token":"Abcde12345", "token_type": "Bearer"}'
      ]

      successSpy = sinon.spy()
      failSpy = sinon.spy()
      App.authenticate("pete", "123456").then(successSpy, failSpy)

      expect(successSpy.called).to.be.false 'promise is not yet resolved'
      expect(failSpy.called).to.be.false 'promise is not yet rejected'
      server.respond()
      Test.nextTick ->
        expect(successSpy.calledOnce).to.be.true 'promise is resolved'
        expect(successSpy.calledWith('Abcde12345')).to.be.true 'resolved with token string'
        expect(failSpy.called).to.be.false 'promise is not rejected'

  context 'when server responds with 400 Bad Request', ->
    it 'invokes callback with error', ->
      server.respondWith "POST", "/api/v0/oauth/token", [
        400
        { 'Content-Type': 'application/json; charset=utf-8' }
        '{"error":"invalid_grant", "error_description": "incorrect username or password"}'
      ]

      successSpy = sinon.spy()
      failSpy = sinon.spy()
      App.authenticate("pete", "123456").then(successSpy, failSpy)

      expect(successSpy.called).to.be.false 'promise is not yet resolved'
      expect(failSpy.called).to.be.false 'promise is not yet rejected'
      server.respond()
      Test.nextTick ->
        expect(successSpy.called).to.be.false 'promise is not resolved'
        expect(failSpy.calledOnce).to.be.true 'promise is rejected'
        err = failSpy.args[0][0]
        expect(err).to.be.instanceof Error, 'error is passed'
        expect(err.type).to.equal 'invalid_grant', 'err.type is set correctly'
        expect(err.message).to.equal 'incorrect username or password', 'err.message is set correctly'
