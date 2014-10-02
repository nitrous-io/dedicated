describe 'App.postError', ->
  beforeEach ->
    sinon.stub App.snackbar, 'post'

  afterEach ->
    App.snackbar.post.restore()

  context 'when err is empty', ->
    it 'calls snackbar.post with default error message', ->
      App.postError(null)
      expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
      expect(App.snackbar.post.calledWith('Something went wrong')).to.be.true 'snackbar.post is called with err'

    context 'when a custom default message is given', ->
      it 'calls snackbar.post with error and custom default message', ->
        App.postError(null, 'oh noes')
        expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
        expect(App.snackbar.post.calledWith('Oh noes')).to.be.true 'snackbar.post is called with custom error message'

  context 'when err is string', ->
    it 'calls snackbar.post with given err', ->
      App.postError('some error')
      expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
      expect(App.snackbar.post.calledWith('Some error')).to.be.true 'snackbar.post is called with err'

  context 'when err is object', ->
    context 'when err contains error_description', ->
      it 'calls snackbar.post with error_description', ->
        App.postError(error: 'fail', error_description: 'something wrong', 'oh noes')
        expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
        expect(App.snackbar.post.calledWith('Something wrong')).to.be.true 'snackbar.post is called with error_description text'

    context 'when err does not contain error_description but error', ->
      it 'calls snackbar.post with error', ->
        App.postError(error: 'fail')
        expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
        expect(App.snackbar.post.calledWith('Something went wrong: fail')).to.be.true 'snackbar.post is called with default error message and error'

      context 'when a custom default message is given', ->
        it 'calls snackbar.post with error and custom default message', ->
          App.postError(error: 'fail', 'oh noes')
          expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
          expect(App.snackbar.post.calledWith('Oh noes: fail')).to.be.true 'snackbar.post is called with custom error message and error'

      context 'when err contains error hash', ->
        it 'calls snackbar.post with error and error hash', ->
          App.postError(error: 'fail', error_hash: 'cafebabedeadbeef')
          expect(App.snackbar.post.calledOnce).to.be.true 'snackbar.post is called'
          expect(App.snackbar.post.calledWith('Something went wrong: fail (Error code: cafebabedeadbeef)')).to.be.true 'snackbar.post is called with error and error_hash'
