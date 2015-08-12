App.ContainerController = Em.ObjectController.extend
  needs: ['orgs']

  orgs: Em.computed.alias 'controllers.orgs.collection'

  org: (->
    if orgId = @get 'model.host.orgId'
      @get('orgs')?.findBy('id', orgId)
  ).property 'orgs.@each', 'model.host.orgId'
