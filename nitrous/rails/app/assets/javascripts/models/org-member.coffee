App.OrgMember = klass = App.NewModel.extend
  id: null
  orgId: null
  orgSlug: null
  userId: null
  username: null
  email: null
  name: null
  role: null

  createdAt: null
  updatedAt: null

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=72"
  ).property 'email'

klass.reopenClass
  collectionUrl: '/api/v0/orgs/:orgSlug/members'
  collectionRoot: 'members'
  recordRoot: 'member'

  deserialize: (data={}) ->
    id: data.id
    orgId: data.org_id
    orgSlug: data.org_slug
    userId: data.user_id
    username: data.username
    email: data.email
    name: data.name
    role: data.role
    createdAt: new Date(data.created_at) if data.created_at != undefined
    updatedAt: new Date(data.updated_at) if data.updated_at != undefined
