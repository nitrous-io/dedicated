App.SidebarController = Em.Controller.extend
  needs: ['orgs', 'org']

  actions:
    createContainer: (host)->
      @transitionToRoute 'containers.new', queryParams: { preselectHost: host.get('slug') }