App.Org = klass = App.NewModel.extend
  id: null
  slug: null
  name: null
  email: null
  billingProfile: null

  createdAt: null
  updatedAt: null

  members: null

  serialize: ->
    slug: @get 'slug'
    name: @get 'name'
    email: @get 'email'

  recordUrl: (->
    if slug = @get 'slug'
      "/api/v0/orgs/#{slug}"
  ).property 'slug'

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=40"
  ).property 'email'

  fetchMembers: ->
    p = App.OrgMember.fetch(null, orgSlug: @get('slug'))
    p.then (members) =>
      @set 'members', members
    p

  subscriptions: (->
    if slug = @get('slug')
      App.OrgSubscription.fetch(orgSlug: slug).value
  ).property 'slug'

  invoices: (->
    if slug = @get('slug')
      App.OrgInvoice.fetch(orgSlug: slug).value
  ).property 'slug'

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
    billingProfile: App.OrgBillingProfile.create(orgSlug: data.slug).load(data.billing_profile) if data.billing_profile != undefined
