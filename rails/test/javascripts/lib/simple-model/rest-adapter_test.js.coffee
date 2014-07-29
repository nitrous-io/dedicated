attr = SM.attr

TestModel = klass = SM.Model.extend
  id: attr('number')
  slug: attr()
  name: attr()
  age: attr('number')
  createdAt: attr('date', as: 'created_at', serialize: false)

klass.reopenClass
  adapter: SM.RESTAdapter.create()
  url: '/foo'

describe 'SM.RESTAdapter', ->
  model = server = null

  beforeEach ->
    server = sinon.fakeServer.create()
    Em.notifyPropertyChange TestModel, '_cache' # reset cache

  afterEach ->
    server.restore()

  describe '#fetch', ->
    context 'fetching a single record', ->
      itWorks = (url, doAction) ->
        it "fetches the record and returns record obj", ->
          server.respondWith 'GET', url, [
            200
            { 'Content-Type': 'application/json; charset=utf-8' }
            '{"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"}'
          ]

          spy = sinon.spy()
          spy2 = sinon.spy()
          record = doAction()
          record.one 'didLoad', spy
          record.one 'didFailToLoad', spy2

          expect(record).to.be.instanceof TestModel, 'fetch returns a record immediately'
          expect(record.get 'isLoaded').to.be.false 'record is not yet loaded'
          expect(record.get 'isLoading').to.be.true 'record is loading'
          expect(spy.calledWith(record)).to.be.false "didLoad event hasn't been triggered"

          server.respond()
          expect(record.get 'isLoading').to.be.false 'record has finished loading'
          expect(record.get 'isLoaded').to.be.true 'record is loaded'
          expect(record.get 'id').to.equal 123, 'id is loaded'
          expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
          expect(record.get 'name').to.equal 'Foo', 'name is loaded'
          expect(record.get 'age').to.equal 15, 'age is loaded'
          expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
          expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

          expect(spy.calledWith(record, sinon.match.has('responseText'))).to.be.true 'didLoad event did trigger'
          expect(spy2.called).to.be.false 'didFailToLoad event did not trigger'

        context 'when server responds with error', ->
          it 'triggers didFailToLoad event', ->
            server.respondWith 'GET', url, [
              404
              { 'Content-Type': 'application/json; charset=utf-8' }
              '{"error": "not found"}'
            ]

            spy = sinon.spy()
            spy2 = sinon.spy()
            record = doAction()
            record.one 'didLoad', spy
            record.one 'didFailToLoad', spy2

            server.respond()
            expect(record.get 'isLoading').to.be.false 'record has stopped loading'
            expect(record.get 'isLoaded').to.be.false 'record is not loaded'
            expect(spy.called).to.be.false 'didLoad event is not triggered'
            expect(spy2.calledWith(record, sinon.match.has('responseText'))).to.be.true 'didFailToLoad event did trigger'

        context 'when rootKey is defined', ->
          beforeEach ->
            klass.reopenClass(rootKey: "foo")

          afterEach ->
            klass.reopenClass(rootKey: null)

          it 'takes data from a given rootKey in the json', ->
            server.respondWith 'GET', url, [
              200
              { 'Content-Type': 'application/json; charset=utf-8' }
              '{"foo": {"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"}, "slug": "ignorethis"}'
            ]

            record = doAction()
            server.respond()
            expect(record.get 'id').to.equal 123, 'id is loaded'
            expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
            expect(record.get 'name').to.equal 'Foo', 'name is loaded'
            expect(record.get 'age').to.equal 15, 'age is loaded'
            expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
            expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

      context 'when it is not a singular resource (fetch with primary key)', ->
        itWorks('/foo/123', -> TestModel.fetch(123))

      context 'when it is a singular resource', ->
        beforeEach ->
          klass.reopenClass(singular: true)

        afterEach ->
          klass.reopenClass(singular: false)

        itWorks('/foo', -> TestModel.fetch())

    context 'fetching multiple records', ->
      it "fetches records and returns record obj", ->
        server.respondWith 'GET', '/foo', [
          200
          { 'Content-Type': 'application/json; charset=utf-8' }
          '''
          [
            {"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"},
            {"id": 124, "slug": "bar", "name": "Bar", "age": 20, "created_at": "2014-07-30T00:00:00.000Z"}
          ]
          '''
        ]

        spy = sinon.spy()
        spy2 = sinon.spy()
        recordArr = TestModel.fetch()
        recordArr.one 'didLoad', spy
        recordArr.one 'didFailToLoad', spy2

        expect(recordArr).to.be.instanceof SM.RecordArray, 'fetch returns a record array immediately'
        expect(recordArr.get 'model').to.equal TestModel, 'record array is for TestModel'
        expect(recordArr.get 'isLoaded').to.be.false 'record array is not yet loaded'
        expect(recordArr.get 'isLoading').to.be.true 'record array is loading'
        expect(spy.calledWith(recordArr)).to.be.false "didLoad event hasn't been triggered"

        server.respond()
        record1 = recordArr.objectAt(0)
        record2 = recordArr.objectAt(1)

        expect(record1.get 'isLoading').to.be.false 'record has finished loading'
        expect(record1.get 'isLoaded').to.be.true 'record is loaded'
        expect(record1.get 'id').to.equal 123, 'id is loaded'
        expect(record1.get 'slug').to.equal 'foo', 'slug is loaded'
        expect(record1.get 'name').to.equal 'Foo', 'name is loaded'
        expect(record1.get 'age').to.equal 15, 'age is loaded'
        expect(record1.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
        expect(TestModel._findInCache(123)).to.equal record1, 'record is cached'

        expect(record2.get 'isLoading').to.be.false 'record2 has finished loading'
        expect(record2.get 'isLoaded').to.be.true 'record2 is loaded'
        expect(record2.get 'id').to.equal 124, 'id is loaded'
        expect(record2.get 'slug').to.equal 'bar', 'slug is loaded'
        expect(record2.get 'name').to.equal 'Bar', 'name is loaded'
        expect(record2.get 'age').to.equal 20, 'age is loaded'
        expect(record2.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 30)), 'createdAt is loaded'
        expect(TestModel._findInCache(124)).to.equal record2, 'record is cached'

        expect(spy.calledWith(recordArr, sinon.match.has('responseText'))).to.be.true 'didLoad event did trigger'
        expect(spy2.called).to.be.false 'didFailToLoad event did not trigger'

      context 'when server responds with error', ->
        it 'triggers didFailToLoad event', ->
          server.respondWith 'GET', '/foo', [
            404
            { 'Content-Type': 'application/json; charset=utf-8' }
            '{"error": "not found"}'
          ]

          spy = sinon.spy()
          spy2 = sinon.spy()
          recordArr = TestModel.fetch()
          recordArr.one 'didLoad', spy
          recordArr.one 'didFailToLoad', spy2

          server.respond()
          expect(recordArr.get 'isLoading').to.be.false 'record array has stopped loading'
          expect(recordArr.get 'isLoaded').to.be.false 'record array is not loaded'
          expect(spy.called).to.be.false 'didLoad event is not triggered'
          expect(spy2.calledWith(recordArr, sinon.match.has('responseText'))).to.be.true 'didFailToLoad event did trigger'

      context 'when collectionKey is defined', ->
        beforeEach ->
          klass.reopenClass(collectionKey: "foo")

        afterEach ->
          klass.reopenClass(collectionKey: null)

        it 'takes data from a given collectionKey in the json', ->
          server.respondWith 'GET', '/foo', [
            200
            { 'Content-Type': 'application/json; charset=utf-8' }
            '''
              {"foo": [
                {"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"},
                {"id": 124, "slug": "bar", "name": "Bar", "age": 20, "created_at": "2014-07-30T00:00:00.000Z"}
              ], "slug": "ignorethis"}
            '''
          ]

          recordArr = TestModel.fetch()
          server.respond()
          record1 = recordArr.objectAt(0)
          record2 = recordArr.objectAt(1)

          expect(record1.get 'id').to.equal 123, 'id is loaded'
          expect(record1.get 'slug').to.equal 'foo', 'slug is loaded'
          expect(record1.get 'name').to.equal 'Foo', 'name is loaded'
          expect(record1.get 'age').to.equal 15, 'age is loaded'
          expect(record1.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
          expect(TestModel._findInCache(123)).to.equal record1, 'record is cached'

          expect(record2.get 'id').to.equal 124, 'id is loaded'
          expect(record2.get 'slug').to.equal 'bar', 'slug is loaded'
          expect(record2.get 'name').to.equal 'Bar', 'name is loaded'
          expect(record2.get 'age').to.equal 20, 'age is loaded'
          expect(record2.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 30)), 'createdAt is loaded'
          expect(TestModel._findInCache(124)).to.equal record2, 'record is cached'
