class FakeWebSocket
  constructor: ->
    @_target = document.createDocumentFragment()

  close: ->
    evt = document.createEvent('CustomEvent')
    evt.initCustomEvent('close')
    @dispatchEvent(evt)
    @onclose()

  send: ->

  onopen: ->
  onclose: ->
  onerror: ->
  onmessage: ->

  addEventListener: ->
    @_target.addEventListener.apply(@_target, arguments)

  removeEventListener: ->
    @_target.removeEventListener.apply(@_target, arguments)

  dispatchEvent: ->
    @_target.dispatchEvent.apply(@_target, arguments)

describe 'SocketConnection', ->
  beforeEach ->
    sinon.stub window, 'WebSocket', FakeWebSocket

  afterEach ->
    window.WebSocket.restore()

  describe 'constructor', ->
    it 'sets url and protocols', ->
      conn = new SocketConnection('wss://example.com/ws', ['foo', 'bar'])
      expect(conn.url).to.equal 'wss://example.com/ws', 'url attr is set'
      expect(conn.protocols).to.eql ['foo', 'bar'], 'protocols attr is set'
      expect(conn.ws).to.be.null 'underlying websocket instance is not yet initialized'

  describe 'instance', ->
    conn = null

    beforeEach ->
      conn = new SocketConnection('wss://example.com/ws')

    describe '#open', ->
      beforeEach ->
        sinon.stub conn, 'onbeforeopen'

      context 'when nobeforeopen argument is not passed', ->
        it 'calls onbeforeopen', ->
          conn.open()
          expect(conn.onbeforeopen.called).to.be.true 'onbeforeopen is called'

      context 'when nobeforeopen argument is true', ->
        it 'does not call onbeforeopen', ->
          conn.open(true)
          expect(conn.onbeforeopen.called).to.be.false 'onbeforeopen is not called'

      context 'when url is available', ->
        it 'initializes underlying websocket instance', ->
          conn.open()
          expect(conn.ws).to.be.instanceof WebSocket, 'websocket is initialized'
          expect(WebSocket.calledWith(conn.url, conn.protocols)).to.be.true 'websocket is initialized with correct params'

        it "forwards underlying websocket instance's onopen, onclose, onmessage, and onerror", ->
          conn.open()

          evt = sinon.spy()
          sinon.stub conn, 'onopen'
          conn.ws.onopen(evt)
          expect(conn.onopen.calledOnce && conn.onopen.calledWith(evt)).to.be.true 'onopen is called with correct event'

          sinon.stub conn, 'onclose'
          conn.ws.onclose(evt)
          expect(conn.onclose.calledOnce && conn.onclose.calledWith(evt)).to.be.true 'onclose is called with correct event'

          sinon.stub conn, 'onmessage'
          conn.ws.onmessage(evt)
          expect(conn.onmessage.calledOnce && conn.onmessage.calledWith(evt)).to.be.true 'onmessage is called with correct event'

          sinon.stub conn, 'onerror'
          conn.ws.onerror(evt)
          expect(conn.onerror.calledOnce && conn.onerror.calledWith(evt)).to.be.true 'onerror is called with correct event'

      context 'when url is not available', ->
        beforeEach ->
          conn.url = null

        it 'does not initialize websocket', ->
          conn.open()
          expect(conn.ws).to.be.null 'websocket is null'

    describe '#send', ->
      beforeEach ->
        conn.ws = sinon.stub(send: ->)

      it 'delegates to underlying websocket instance', ->
        conn.send('hello world')
        expect(conn.ws.send.calledOnce && conn.ws.send.calledWith('hello world')).to.be.true 'arguments to send are correctly forwarded'

    describe '#close', ->
      beforeEach ->
        conn.ws = sinon.stub(close: ->)

      it 'closes websocket connection, and prohibits auto-reconnection', ->
        conn.close()
        expect(conn.ws.close.calledOnce).to.be.true 'ws.close is called once'
        expect(conn.ws._noRetry).to.be.true '_noRetry is flag is set to be true'

      context 'when true is passed as an argument', ->
        it 'closes websocket connection, but allows it to be automatically reconnected', ->
          conn.close(true)
          expect(conn.ws.close.calledOnce).to.be.true 'ws.close is called once'
          expect(conn.ws._noRetry).to.be.false '_noRetry is flag is set to be false'

    describe 'auto reconnect', ->
      beforeEach ->
        conn.open()

      context 'when close event is dispatched', ->
        beforeEach ->
          sinon.stub conn, 'retry'

        context 'if it is due to user invoking close()', ->
          it 'does not attempt to auto reconnect', ->
            conn.close()
            expect(conn.retry.called).to.be.false 'retry is not called'

        context 'if is not invoked by user', ->
          it 'schedules auto reconnect', ->
            conn.ws.close()
            expect(conn.retry.called).to.be.true 'retry is called'

      describe '#retry', ->
        beforeEach ->
          sinon.stub window, 'setTimeout'
          sinon.stub conn, 'open'

        afterEach ->
          window.setTimeout.restore()

        it 'increments retries', ->
          conn.retries = 5
          conn.retry()
          expect(conn.retries).to.equal 6, 'retries is incremented'

        context 'when retries <= 5', ->
          beforeEach ->
            conn.retries = 5
            conn.retry()

          it 'attempts to reopen connection after 2 seconds', ->
            expect(conn.open.called).to.be.false 'open is not yet called'
            expect(window.setTimeout.calledOnce && window.setTimeout.calledWith(sinon.match.func, 2000)).to.be.true 'timeout after 2s'
            window.setTimeout.args[0][0]()
            expect(conn.open.called).to.be.true 'open is called'

        context 'when 5 < retries <= 10', ->
          beforeEach ->
            conn.retries = 6
            conn.retry()

          it 'attempts to reopen connection after 3 seconds', ->
            expect(conn.open.called).to.be.false 'open is not yet called'
            expect(window.setTimeout.calledOnce && window.setTimeout.calledWith(sinon.match.func, 3000)).to.be.true 'timeout after 3s'
            window.setTimeout.args[0][0]()
            expect(conn.open.called).to.be.true 'open is called'

        context 'when retries > 10', ->
          beforeEach ->
            conn.retries = 11
            conn.retry()

          it 'attempts to reopen connection after 5 seconds', ->
            expect(conn.open.called).to.be.false 'open is not yet called'
            expect(window.setTimeout.calledOnce && window.setTimeout.calledWith(sinon.match.func, 5000)).to.be.true 'timeout after 5s'
            window.setTimeout.args[0][0]()
            expect(conn.open.called).to.be.true 'open is called'

          context 'when reconnect is successful', ->
            beforeEach ->
              evt = document.createEvent('CustomEvent')
              evt.initCustomEvent('open')
              conn.ws.dispatchEvent(evt)

            it 'resets retry count', ->
              expect(conn.retries).to.equal 0, 'retry count is reset'
