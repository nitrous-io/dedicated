App.Org = klass = App.NewModel.extend
  id: null
  slug: null
  name: null
  email: null

  createdAt: null
  updatedAt: null

  serialize: ->
    slug: @get 'slug'
    name: @get 'name'
    email: @get 'email'

klass.reopenClass
  collectionUrl: '/api/v0/orgs'
  collectionRoot: 'orgs'
  recordRoot: 'org'

  deserialize: (data={}) ->
    id: data.id
    slug: data.slug
    name: data.name
    email: data.email
    createdAt: new Date(data.created_at) if data.created_at != undefined
    updatedAt: new Date(data.updated_at) if data.updated_at != undefined
