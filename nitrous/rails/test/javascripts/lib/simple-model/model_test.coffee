attr = SM.attr

TestModel = SM.Model.extend
  id: attr('number')
  firstName: attr(as: 'first_name')
  lastName: attr(as: 'last_name')
  bio: attr()
  age: attr('number', default: 12)
  data: attr('json')
  bornAt: attr('date', as: 'born_at')
  createdAt: attr('date', as: 'created_at', serialize: false)

  fullName: (->
    "#{@get 'firstName'} #{@get 'lastName'}"
  ).property 'firstName', 'lastName'

describe 'SM.Model', ->
  model = null

  beforeEach ->
    model = TestModel.create()
    Em.notifyPropertyChange TestModel, '_cache' # reset cache

  describe 'attributes', ->
    describe 'MO.attr', ->
      it 'defines attributes', ->
        attrs = Em.get TestModel, '_attrs'
        expect(attrs.keys.list).to.eql ['id', 'firstName', 'lastName', 'bio', 'age', 'data', 'bornAt', 'createdAt'], 'has 7 attrs'
        expect(attrs.get('firstName').type).to.equal 'string', 'firstName is string'
        expect(attrs.get('lastName').type).to.equal 'string', 'lastName is string'
        expect(attrs.get('bio').type).to.equal 'string', 'bio is string'
        expect(attrs.get('age').type).to.equal 'number', 'age is number'
        expect(attrs.get('bornAt').type).to.equal 'date', 'bornAt is date'
        expect(attrs.get('createdAt').type).to.equal 'date', 'createdAt is date'

    describe '._getKey', ->
      it 'returns the serialization key for a given attribute', ->
        expect(TestModel._getKey 'firstName').to.equal 'first_name', 'firstName is serialized as first_name'
        expect(TestModel._getKey 'lastName').to.equal 'last_name', 'lastName is serialized as last_name'
        expect(TestModel._getKey 'bio').to.equal 'bio', 'bio is serialized as bio'
        expect(TestModel._getKey 'age').to.equal 'age', 'age is serialized as age'
        expect(TestModel._getKey 'bornAt').to.equal 'born_at', 'bornAt is serialized as born_at'
        expect(TestModel._getKey 'createdAt').to.equal 'created_at', 'createdAt is serialized as created_at'

    describe 'get/set', ->
      it 'works as expected', ->
        model.set 'firstName', 'Pete'
        model.set 'lastName', 'Kim'
        expect(model.get 'firstName').to.equal 'Pete', 'setting a property works'
        expect(model.get 'lastName').to.equal 'Kim', 'setting another property works'
        expect(model.get 'fullName').to.equal 'Pete Kim', 'a computed property works'
        expect(model.get 'age').to.equal 12, 'default value is set'

  describe 'cache', ->
    model2 = null

    beforeEach ->
      model.setProperties
        id: 1
        firstName: 'Pete'
        lastName: 'Kim'
      model2 = TestModel.create
        id: 2
        firstName: 'Pino'
        lastName: 'cchio'

    describe '#_saveToCache', ->
      context 'when primary key is set', ->
        context 'when the cache does not already contain the record', ->
          it 'saves the record to cache', ->
            cache = Em.get(TestModel, '_cache')
            expect(cache.length).to.equal 0, 'nothing is cached yet'
            model._saveToCache()
            model2._saveToCache()
            expect(cache.length).to.equal 2, 'two items are cached'
            expect(cache.get(1)).to.equal model, 'model is cached'
            expect(cache.get(2)).to.equal model2, 'model2 is cached'

        context 'when the cache already contains the record', ->
          oldModel = oldModel2 = null

          beforeEach ->
            oldModel = TestModel.create(id: 1)
            oldModel2 = TestModel.create(id: 2)
            oldModel._saveToCache()
            oldModel2._saveToCache()

          it 'returns the last record saved', ->
            cache = Em.get(TestModel, '_cache')
            expect(cache.get(1)).to.equal oldModel, 'oldModel is cached'
            expect(cache.get(2)).to.equal oldModel2, 'oldModel2 is cached'
            model._saveToCache()
            model2._saveToCache()
            expect(cache.get(1)).to.equal model, 'model is cached'
            expect(cache.get(2)).to.equal model2, 'model2 is cached'

      context 'when primary key is not set', ->
        beforeEach ->
          model.set 'id', null
          model2.set 'id', null

        it 'does nothing', ->
          model._saveToCache()
          model2._saveToCache()
          expect(Em.get(TestModel, '_cache').length).to.equal 0, 'nothing is in the cache'

      context 'when model is singular', ->
        beforeEach ->
          TestModel.reopenClass(singular: true)
          # primary key is not relevant
          model.set 'id', null
          model2.set 'id', null

        afterEach ->
          TestModel.reopenClass(singular: false)

        it 'only keeps the last item saved', ->
          cache = Em.get(TestModel, '_cache')
          model._saveToCache()
          expect(cache.length).to.equal 1, "cache has one item"
          expect(cache.get(cache.keys.list[0])).to.equal model
          model2._saveToCache()
          expect(cache.length).to.equal 1, "cache still has only one item"
          expect(cache.get(cache.keys.list[0])).to.equal model2

    describe '._findInCache', ->
      model3 = null

      context 'when model is not a singular resource', ->
        beforeEach ->
          model._saveToCache()
          model2._saveToCache()
          model3 = TestModel.create
            id: 3
            firstName: 'Foo'
            lastName: 'Bar'

        context 'when the cache contains the record with a given primary key', ->
          it 'returns the record', ->
            expect(TestModel._findInCache(1)).to.equal model, 'model is returned'
            expect(TestModel._findInCache(2)).to.equal model2, 'model2 is returned'

        context 'when the cache does not contain the record with a given primary key', ->
          it 'returns undefined', ->
            expect(TestModel._findInCache(3)).to.be.undefined 'model3 is not returned (not cached)'

      context 'when model is a singular resource', ->
        beforeEach ->
          TestModel.reopenClass(singular: true)
          model._saveToCache()
          model2._saveToCache()

        afterEach ->
          TestModel.reopenClass(singular: false)

        it 'ignores primary key and always returns the last item saved', ->
          expect(TestModel._findInCache(1)).to.equal model2, 'model2 is returned'
          expect(TestModel._findInCache(2)).to.equal model2, 'model2 is returned'
          expect(TestModel._findInCache()).to.equal model2, 'model2 is returned'

    describe '._removeFromCache', ->
      beforeEach ->
        model._saveToCache()
        model2._saveToCache()

      it 'removes the record from the cache', ->
        expect(TestModel._findInCache(1)).not.to.be.undefined 'model2 is not yet uncached'
        TestModel._removeFromCache(1)
        expect(TestModel._findInCache(1)).to.be.undefined 'model is uncached'
        expect(TestModel._findInCache(2)).not.to.be.undefined 'model2 is not yet uncached'
        TestModel._removeFromCache(2)
        expect(TestModel._findInCache(2)).to.be.undefined 'model2 is uncached'

    describe '.newRecord', ->
      context 'when a record with a given primary key already exists in the cache', ->
        beforeEach ->
          model._saveToCache()
          model2._saveToCache()

        it 'reuses the record object in the cache', ->
          m = TestModel.newRecord(id: 1, firstName: 'Foo')
          expect(m).to.equal model, 'model is reused'
          expect(m.get 'firstName').to.equal 'Foo', 'new attrs are set on the reused obj'
          expect(m.get 'lastName').to.equal 'Kim', 'previous attrs are preserved'

          m2 = TestModel.newRecord(id: 2)
          expect(m2).to.equal model2, 'model2 is reused'

      context 'when a record with a given primary key does not exist in the cache', ->
        it 'creates a new record', ->
          m = TestModel.newRecord(id: 1, firstName: 'Foo')
          expect(m).not.to.equal model, 'model is not reused'
          expect(m.get 'firstName').to.equal 'Foo', 'new attrs are set on the new record'
          expect(m.get 'lastName').to.be.undefined 'new record does not know about prev attrs'

          m2 = TestModel.newRecord()
          expect(m2).not.to.equal model, 'model is not reused'
          expect(m2).not.to.equal model2, 'model2 is not reused'
          expect(m2).to.be.instanceof TestModel, 'returns a new instance of TestModel'

      context 'for a singular resource', ->
        beforeEach ->
          TestModel.reopenClass(singular: true)
          model._saveToCache()

        afterEach ->
          TestModel.reopenClass(singular: false)

        it 'always uses the same record', ->
          m = TestModel.newRecord()
          m2 = TestModel.newRecord()
          expect(m).to.equal model, 'reuses the cached record'
          expect(m2).to.equal model, 'reuses the cached record'

  describe 'serialization', ->
    beforeEach ->
      model.setProperties
        firstName: 'Pino'
        lastName: 'cchio'
        bio: 'I wanna be a real boy.'
        age: 1
        data:
          foo: 'bar'
          baz: 123
        bornAt: new Date(Date.UTC(1883, 0, 1))
        createdAt: new Date(Date.UTC(2014, 6, 28))

    describe '#serialize', ->
      it 'serializes correctly', ->
        serialized = model.serialize()
        expect(serialized).to.eql(
          first_name: 'Pino'
          last_name: 'cchio'
          bio: 'I wanna be a real boy.'
          age: 1
          data: '{"foo":"bar","baz":123}'
          born_at: '1883-01-01T00:00:00.000Z'
        , 'serialized with correct keys and value conversions')
        expect(serialized.created_at).not.to.be.defined 'created_at is not serialized (serialize: false)'

    describe '.deserialize', ->
      it 'deserializes correctly', ->
        data = TestModel.deserialize
          first_name: 'Foo'
          last_name: 'Bar'
          bio: 'foobar!!!'
          age: 20
          data: '{"foo":"bar","baz":123}'
          born_at: '2014-07-28T00:00:00.000Z'
        expect(data.firstName).to.equal 'Foo', 'first_name is correctly deserialized'
        expect(data.lastName).to.equal 'Bar', 'last_name is correctly deserialized'
        expect(data.bio).to.equal 'foobar!!!', 'bio is correctly deserialized'
        expect(data.age).to.equal 20, 'age is correctly deserialized'
        expect(data.data).to.eql { foo: 'bar', baz: 123 }, 'data is correctly desrialized'
        expect(data.bornAt.valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'born_at is correctly deserialized'

      it 'deserializes other types of values correctly', ->
        data = TestModel.deserialize
          first_name: ''
          last_name: 123
          bio: null
          age: '20'
          data: ''
          born_at: 1406505600000
          extra: 'stuff'
        expect(data.firstName).to.equal '', 'first_name is correctly deserialized (blank)'
        expect(data.lastName).to.equal '123', 'last_name is correctly deserialized (number -> string)'
        expect(data.bio).to.be.null 'bio is correctly deserialized (null)'
        expect(data.age).to.equal 20, 'age is correctly deserialized (string -> number)'
        expect(data.data).to.be.null 'data is correctly deserialized (null)'
        expect(data.bornAt.valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'born_at is correctly deserialized'
        expect(data.extra).not.to.be.defined 'extra is not deserialized'

    describe '#load', ->
      it 'deserializes and loads data and sets loaded to true', ->
        expect(model.get 'loaded').to.be.false 'before load, loaded is false'
        model.load
          id: 123
          first_name: 'Foo'
          last_name: 'Bar'
          bio: 'foobar!!!'
          age: 20
          born_at: '2014-07-28T00:00:00.000Z'
        expect(model.get 'loaded').to.be.true 'after load, loaded is true'
        expect(model.get 'firstName').to.equal 'Foo', 'first_name is correctly loaded'
        expect(model.get 'lastName').to.equal 'Bar', 'last_name is correctly loaded'
        expect(model.get 'bio').to.equal 'foobar!!!', 'bio is correctly loaded'
        expect(model.get 'age').to.equal 20, 'age is correctly loaded'
        expect(model.get('bornAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'born_at is correctly loaded'
        expect(model.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'createdAt is not updated (missing)'

      it 'caches the record', ->
        expect(TestModel._findInCache(123)).to.be.undefined 'cache does not yet contain the record'
        model.load(id: 123)
        expect(TestModel._findInCache(123)).to.equal model, 'the record is cached'

      context 'when deserialize option is false', ->
        it 'loads the data without deserializing', ->
          expect(model.get 'loaded').to.be.false 'before load, loaded is false'
          model.load(
            firstName: 'Foo'
            lastName: 'Bar'
            bio: 'foobar!!!'
            age: 20
            bornAt: new Date(Date.UTC(2014, 6, 28))
          , deserialize: false)
          expect(model.get 'loaded').to.be.true 'after load, loaded is true'
          expect(model.get 'firstName').to.equal 'Foo', 'first_name is correctly loaded'
          expect(model.get 'lastName').to.equal 'Bar', 'last_name is correctly loaded'
          expect(model.get 'bio').to.equal 'foobar!!!', 'bio is correctly loaded'
          expect(model.get 'age').to.equal 20, 'age is correctly loaded'
          expect(model.get('bornAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'born_at is correctly loaded'
          expect(model.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 28)), 'createdAt is not updated (missing)'
