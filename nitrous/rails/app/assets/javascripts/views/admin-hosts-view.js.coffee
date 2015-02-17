App.AdminHostsView = Em.View.extend
  elementId: 'admin-hosts-view'
  classNames: ['admin', 'ui', 'card']

  actions:
    updateHosts: (hosts)->
      selectedHosts = hosts.filterBy('selected', true)
      App.trigger 'show-update-hosts', selectedHosts
