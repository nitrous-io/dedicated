App.HostController = Em.ObjectController.extend
  needs: ['orgs']

  orgs: Em.computed.alias 'controllers.orgs.collection'

  org: (->
    if orgId = @get 'model.orgId'
      console.log orgId
      @get('orgs')?.findBy('id', orgId)
  ).property 'orgs.@each', 'model.orgId'
