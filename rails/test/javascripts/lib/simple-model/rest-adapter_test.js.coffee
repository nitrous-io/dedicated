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

  describe 'fetch', ->
    context 'fetching a single record', ->
      itWorks = (url, doAction) ->
        context 'when request succeeds', ->
          it 'returns a promise that is resolved with the fetched record', ->
            server.respondWith 'GET', url, [
              200
              { 'Content-Type': 'application/json; charset=utf-8' }
              '{"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"}'
            ]

            successSpy = sinon.spy()
            failSpy = sinon.spy()
            promise = doAction()
            promise.then(successSpy, failSpy)
            promiseValue = promise.value

            expect(successSpy.called).to.be.false 'promise is not yet resolved'
            expect(failSpy.called).to.be.false 'promise is not yet rejected'
            server.respond()
            Test.nextTick ->
              expect(successSpy.calledOnce).to.be.true 'promise is resolved'
              expect(failSpy.called).to.be.false 'promise is not rejected'

              record = successSpy.args[0][0]
              expect(record).to.be.instanceof TestModel, 'resolved object is a TestModel record'
              expect(record).to.equal promiseValue, "resolved record is the same instance as the promise's previously returned value"
              expect(record.get 'id').to.equal 123, 'id is loaded'
              expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
              expect(record.get 'name').to.equal 'Foo', 'name is loaded'
              expect(record.get 'age').to.equal 15, 'age is loaded'
              expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
              expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

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

              successSpy = sinon.spy()
              failSpy = sinon.spy()
              promise = doAction()
              promise.then(successSpy, failSpy)
              promiseValue = promise.value

              expect(successSpy.called).to.be.false 'promise is not yet resolved'
              expect(failSpy.called).to.be.false 'promise is not yet rejected'
              server.respond()
              Test.nextTick ->
                expect(successSpy.calledOnce).to.be.true 'promise is resolved'
                expect(failSpy.called).to.be.false 'promise is not rejected'

                record = successSpy.args[0][0]
                expect(record).to.be.instanceof TestModel, 'resolved object is a TestModel record'
                expect(record).to.equal promiseValue, "resolved record is the same instance as the promise's previously returned value"
                expect(record.get 'id').to.equal 123, 'id is loaded'
                expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
                expect(record.get 'name').to.equal 'Foo', 'name is loaded'
                expect(record.get 'age').to.equal 15, 'age is loaded'
                expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
                expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

        context 'when request fails', ->
          it 'returns a promise that is rejected with jqxhr object', ->
            server.respondWith 'GET', url, Test.responseInternalServerError

            successSpy = sinon.spy()
            failSpy = sinon.spy()
            promise = doAction()
            promise.then(successSpy, failSpy)

            expect(successSpy.called).to.be.false 'promise is not yet resolved'
            expect(failSpy.called).to.be.false 'promise is not yet rejected'
            server.respond()
            Test.nextTick ->
              expect(successSpy.called).to.be.false 'promise is not resolved'
              expect(failSpy.calledOnce).to.be.true 'promise is rejected'
              expect(failSpy.calledWithExactly(sinon.match.has('responseText'))).to.be.true 'promise is rejected with jqxhr object'

      context 'when it is not a singular resource (fetch with primary key)', ->
        itWorks('/foo/123', -> TestModel.fetch(123))

        context 'when params are given', ->
          it 'adds a query string to the request', ->
            promise = TestModel.fetch(123, foo: 'bar baz=', baz: 777)
            expect(server.requests.length).to.equal 1
            expect(server.requests[0].url).to.equal '/foo/123?foo=bar%20baz%3D&baz=777'

      context 'when it is a singular resource', ->
        beforeEach ->
          klass.reopenClass(singular: true)

        afterEach ->
          klass.reopenClass(singular: false)

        itWorks('/foo', -> TestModel.fetch())

        context 'when params are given', ->
          it 'adds a query string to the request', ->
            promise = TestModel.fetch(foo: 'bar baz=', baz: 777)
            expect(server.requests.length).to.equal 1
            expect(server.requests[0].url).to.equal '/foo?foo=bar%20baz%3D&baz=777'

    context 'fetching multiple records', ->
      context 'when request succeeds', ->
        it 'returns a promise that is resolved with the fetched record array', ->
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

          successSpy = sinon.spy()
          failSpy = sinon.spy()
          promise = TestModel.fetch()
          promise.then(successSpy, failSpy)
          promiseValue = promise.value

          expect(successSpy.called).to.be.false 'promise is not yet resolved'
          expect(failSpy.called).to.be.false 'promise is not yet rejected'
          server.respond()
          Test.nextTick ->
            expect(successSpy.calledOnce).to.be.true 'promise is resolved'
            expect(failSpy.called).to.be.false 'promise is not rejected'

            recordArr = successSpy.args[0][0]
            expect(recordArr).to.be.instanceof SM.RecordArray, 'fetch returns a record array immediately'
            expect(recordArr.get 'model').to.equal TestModel, 'record array is for TestModel'
            expect(recordArr).to.equal promiseValue, "resolved record array is the same instance as the promise's previously returned value"
            record1 = recordArr.objectAt(0)
            record2 = recordArr.objectAt(1)

            expect(record1).to.be.instanceof TestModel
            expect(record1.get 'id').to.equal 123, 'id is loaded'
            expect(record1.get 'slug').to.equal 'foo', 'slug is loaded'
            expect(record1.get 'name').to.equal 'Foo', 'name is loaded'
            expect(record1.get 'age').to.equal 15, 'age is loaded'
            expect(record1.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
            expect(TestModel._findInCache(123)).to.equal record1, 'record is cached'

            expect(record2).to.be.instanceof TestModel
            expect(record2.get 'id').to.equal 124, 'id is loaded'
            expect(record2.get 'slug').to.equal 'bar', 'slug is loaded'
            expect(record2.get 'name').to.equal 'Bar', 'name is loaded'
            expect(record2.get 'age').to.equal 20, 'age is loaded'
            expect(record2.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 30)), 'createdAt is loaded'
            expect(TestModel._findInCache(124)).to.equal record2, 'record is cached'

        context 'when params are given', ->
          it 'adds a query string to the request', ->
            promise = TestModel.fetch(foo: 'bar baz=', baz: 777)
            expect(server.requests.length).to.equal 1
            expect(server.requests[0].url).to.equal '/foo?foo=bar%20baz%3D&baz=777'

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

            successSpy = sinon.spy()
            failSpy = sinon.spy()
            promise = TestModel.fetch()
            promise.then(successSpy, failSpy)
            promiseValue = promise.value

            expect(successSpy.called).to.be.false 'promise is not yet resolved'
            expect(failSpy.called).to.be.false 'promise is not yet rejected'
            server.respond()
            Test.nextTick ->
              expect(successSpy.calledOnce).to.be.true 'promise is resolved'
              expect(failSpy.called).to.be.false 'promise is not rejected'

              recordArr = successSpy.args[0][0]
              expect(recordArr).to.be.instanceof SM.RecordArray, 'fetch returns a record array immediately'
              expect(recordArr).to.equal promiseValue, "resolved record array is the same instance as the promise's previously returned value"
              expect(recordArr.get 'model').to.equal TestModel, 'record array is for TestModel'
              record1 = recordArr.objectAt(0)
              record2 = recordArr.objectAt(1)

              expect(record1).to.be.instanceof TestModel
              expect(record1.get 'id').to.equal 123, 'id is loaded'
              expect(record1.get 'slug').to.equal 'foo', 'slug is loaded'
              expect(record1.get 'name').to.equal 'Foo', 'name is loaded'
              expect(record1.get 'age').to.equal 15, 'age is loaded'
              expect(record1.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
              expect(TestModel._findInCache(123)).to.equal record1, 'record is cached'

              expect(record2).to.be.instanceof TestModel
              expect(record2.get 'id').to.equal 124, 'id is loaded'
              expect(record2.get 'slug').to.equal 'bar', 'slug is loaded'
              expect(record2.get 'name').to.equal 'Bar', 'name is loaded'
              expect(record2.get 'age').to.equal 20, 'age is loaded'
              expect(record2.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 30)), 'createdAt is loaded'
              expect(TestModel._findInCache(124)).to.equal record2, 'record is cached'

      context 'when request fails', ->
        it 'returns a promise that is rejected with jqxhr object', ->
          server.respondWith 'GET', '/foo', Test.responseInternalServerError

          successSpy = sinon.spy()
          failSpy = sinon.spy()
          promise = TestModel.fetch()
          promise.then(successSpy, failSpy)

          expect(successSpy.called).to.be.false 'promise is not yet resolved'
          expect(failSpy.called).to.be.false 'promise is not yet rejected'
          server.respond()
          Test.nextTick ->
            expect(successSpy.called).to.be.false 'promise is not resolved'
            expect(failSpy.calledOnce).to.be.true 'promise is rejected'
            expect(failSpy.calledWithExactly(sinon.match.has('responseText'))).to.be.true 'promise is rejected with jqxhr object'

  describe 'create record', ->
    record = null

    beforeEach ->
      record = TestModel.create(slug: 'foo', name: 'Foo', age: 15)

    itWorks = (url) ->
      context 'when request succeeds', ->
        it 'returns a promise that is resolved with the newly created record', ->
          server.respondWith 'POST', url, [
            201
            { 'Content-Type': 'application/json; charset=utf-8' }
            '{"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"}'
          ]

          successSpy = sinon.spy()
          failSpy = sinon.spy()
          promise = record.save()
          promise.then(successSpy, failSpy)
          promiseValue = promise.value

          expect(successSpy.called).to.be.false 'promise is not yet resolved'
          expect(failSpy.called).to.be.false 'promise is not yet rejected'
          server.respond()
          Test.nextTick ->
            expect(successSpy.calledOnce).to.be.true 'promise is resolved'
            expect(failSpy.called).to.be.false 'promise is not rejected'

            expect(successSpy.args[0][0]).to.equal record
            expect(record).to.be.instanceof TestModel, 'resolved object is a TestModel record'
            expect(record).to.equal promiseValue, "resolved record is the same instance as the promise's previously returned value"
            expect(record.get 'id').to.equal 123, 'id is loaded'
            expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
            expect(record.get 'name').to.equal 'Foo', 'name is loaded'
            expect(record.get 'age').to.equal 15, 'age is loaded'
            expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
            expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

        context 'when rootKey is defined', ->
          beforeEach ->
            klass.reopenClass(rootKey: "foo")

          afterEach ->
            klass.reopenClass(rootKey: null)

          it 'takes data from a given rootKey in the json', ->
            server.respondWith 'POST', url, [
              201
              { 'Content-Type': 'application/json; charset=utf-8' }
              '{"foo": {"id": 123, "slug": "foo", "name": "Foo", "age": 15, "created_at": "2014-07-29T00:00:00.000Z"}, "slug": "ignorethis"}'
            ]

            successSpy = sinon.spy()
            failSpy = sinon.spy()
            promise = record.save()
            promise.then(successSpy, failSpy)
            promiseValue = promise.value

            expect(successSpy.called).to.be.false 'promise is not yet resolved'
            expect(failSpy.called).to.be.false 'promise is not yet rejected'
            server.respond()
            Test.nextTick ->
              expect(successSpy.calledOnce).to.be.true 'promise is resolved'
              expect(failSpy.called).to.be.false 'promise is not rejected'

              expect(successSpy.args[0][0]).to.equal record
              expect(record).to.be.instanceof TestModel, 'resolved object is a TestModel record'
              expect(record).to.equal promiseValue, "resolved record is the same instance as the promise's previously returned value"
              expect(record.get 'id').to.equal 123, 'id is loaded'
              expect(record.get 'slug').to.equal 'foo', 'slug is loaded'
              expect(record.get 'name').to.equal 'Foo', 'name is loaded'
              expect(record.get 'age').to.equal 15, 'age is loaded'
              expect(record.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
              expect(TestModel._findInCache(123)).to.equal record, 'record is cached'

      context 'when request fails', ->
        it 'returns a promise that is rejected with jqxhr object', ->
          server.respondWith 'POST', url, Test.responseInternalServerError

          successSpy = sinon.spy()
          failSpy = sinon.spy()
          promise = record.save()
          promise.then(successSpy, failSpy)

          expect(successSpy.called).to.be.false 'promise is not yet resolved'
          expect(failSpy.called).to.be.false 'promise is not yet rejected'
          server.respond()
          Test.nextTick ->
            expect(successSpy.called).to.be.false 'promise is not resolved'
            expect(failSpy.calledOnce).to.be.true 'promise is rejected'
            expect(failSpy.calledWithExactly(sinon.match.has('responseText'))).to.be.true 'promise is rejected with jqxhr object'

    context 'when createURL is not set', ->
      itWorks('/foo')

    context 'when createURL is set', ->
      beforeEach ->
        klass.reopenClass(createURL: '/foo/create')

      afterEach ->
        klass.reopenClass(createURL: null)

        itWorks('/foo/create')
