App.IdeUpgradeController = Em.ObjectController.extend

  upgrade: ->
    console.log "upgrade"
    @set 'upgrading', true
    containerSlug = @get 'model.slug'
    console.log "containerSlug: ", containerSlug
    hostSlug = @get 'model.host.slug'
    ver  = @get 'model.host.freighterd_version'
    console.log "container: ", containerSlug, " host: ", hostSlug, " freighterd_version: ", ver
    Em.$.ajax(
      type: 'POST'
      data: { freighterd_version : ver }
      url: "/api/v0/hosts/#{hostSlug}/upgradefreighter"
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    ).done((data, status, jqxhr) =>
      console.log "upgraded host: ", hostSlug, " container: ", containerSlug, " to ver: ", ver
      @transitionTo "ide", containerSlug
    ).fail (jqxhr) =>
      @set 'upgrading', false
      App.postError(jqxhr.responseJSON, 'Failed to upgrade the workspace! Please contact support')
