attr = SM.attr

TestModel = klass = SM.Model.extend
  id: attr('number')
  name: attr()
  age: attr('number')
  createdAt: attr('date', as: 'created_at', serialize: false)

describe 'SM.RecordArray', ->
  beforeEach ->
    Em.notifyPropertyChange TestModel, '_cache' # reset cache

  describe '#load', ->
    it 'deserializes and loads data and sets loaded to true', ->
      recordArr = SM.RecordArray.create(model: TestModel)
      expect(recordArr.get 'loaded').to.be.false 'before load, loaded is false'
      recordArr.load([
        { id: 123, name: "Foo", age: 15, created_at: "2014-07-29T00:00:00.000Z" }
        { id: 124, name: "Bar", age: 20, created_at: "2014-07-30T00:00:00.000Z" }
      ])
      expect(recordArr.get 'loaded').to.be.true 'after load, loaded is true'
      expect(recordArr.get 'length').to.equal 2, '2 records are loaded'
      record1 = recordArr.objectAt(0)
      record2 = recordArr.objectAt(1)

      expect(record1.get 'id').to.equal 123, 'id is loaded'
      expect(record1.get 'name').to.equal 'Foo', 'name is loaded'
      expect(record1.get 'age').to.equal 15, 'age is loaded'
      expect(record1.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 29)), 'createdAt is loaded'
      expect(TestModel._findInCache(123)).to.equal record1, 'record is cached'
      expect(record1.get 'loaded').to.be.true 'loaded is true'

      expect(record2.get 'id').to.equal 124, 'id is loaded'
      expect(record2.get 'name').to.equal 'Bar', 'name is loaded'
      expect(record2.get 'age').to.equal 20, 'age is loaded'
      expect(record2.get('createdAt').valueOf()).to.equal +new Date(Date.UTC(2014, 6, 30)), 'createdAt is loaded'
      expect(TestModel._findInCache(124)).to.equal record2, 'record is cached'
      expect(record2.get 'loaded').to.be.true 'loaded is true'

    context 'when some records are already in the cache', ->
      record = null

      beforeEach ->
        record = TestModel.create(id: 124, name: "Baz")
        record._saveToCache()

      it 'deserializes and updates cached data', ->
        recordArr = SM.RecordArray.create(model: TestModel)
        recordArr.load([
          { id: 123, name: "Foo" }
          { id: 124, name: "Bar" }
        ])
        expect(recordArr.get 'length').to.equal 2, '2 records are loaded'
        expect(recordArr.objectAt(1)).to.equal record, 'record is reused'

      context 'when record array already contains existing records', ->
        it 'adds new objects and removes deleted objects', ->
          record2 = TestModel.create(id: 125, name: "Qux")
          record2._saveToCache()

          recordArr = SM.RecordArray.create(model: TestModel, content: Em.A([record, record2]))
          recordArr.load([
            { id: 123, name: "Foo" }
            { id: 124, name: "Bar" }
          ])
          expect(recordArr.get 'length').to.equal 2, '2 records are loaded'
          expect(recordArr).to.contain record, 'record array contains record'
          expect(recordArr).not.to.contain record2, 'record array does not contain record2'
