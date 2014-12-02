App.ShoverSocket = App.BaseSocket.extend
  name: 'shover'

  serverURI: null
  serverName: 'shover'

  didConnect: ->
    socket = @get('socket')
    socket.setMaxListeners(100)

    socket.on 'aio:box_updated', (json) ->
      timestamp = json._t
      json = json[App.Box.rootKey]
      if box = App.Box.findById json.id
        lastBoxUpdatedTimestamp = box.get('lastBoxUpdatedTimestamp')
        if !lastBoxUpdatedTimestamp || lastBoxUpdatedTimestamp < timestamp
          box.set 'lastBoxUpdatedTimestamp', timestamp
          box.loadHash json
          trackingData =
            boxTemplate: box.get('boxTemplate.name')
            region: box.get('region.name')

          App.util.mixpanel "box:#{box.get('state')}", trackingData

    socket.on 'aio:snapshot_updated', (json) ->
      json = json[App.Snapshot.rootKey]
      if snapshot = App.Snapshot.findById json.id
        snapshot.loadHash(json)

    socket.on 'aio:subscription_updated', (json) ->
      currentPrice = App.get 'subscription.plan.price'
      json = json.subscription
      if subscription = App.get('subscription') || App.Subscription.create()
        subscription.loadHash(json)
        # Reset flag to allow user to access upgrade/change plan pages again.
        App.set 'currentUser.subscription_pending', false
        App.set 'subscription', subscription

        newPrice = App.get 'subscription.plan.price'
        if window.goog_report_conversion? && (newPrice != currentPrice)
          window.google_conversion_value = newPrice
          window.goog_report_conversion()

    socket.on 'aio:billing_profile_updated', (json) ->
      json = json[App.BillingProfile.rootKey]
      if billingProfile = App.get('billingProfile') || App.BillingProfile.create()
        billingProfile.loadHash(json)
        App.set 'billingProfile', billingProfile

    socket.on 'aio:n2o_updated', (json) ->
      if currentUser = App.get('currentUser')
        n2o = json.n2o
        currentUser.set 'max_n2o', n2o.max
        currentUser.set 'used_n2o', n2o.used

    socket.on 'aio:delinquent_at_updated', (json) ->
      if currentUser = App.get('currentUser')
        currentUser.set 'delinquent_at', json.user.delinquent_at

    socket.on 'aio:read_default_public_key', (json) ->
      json = json[App.Box.rootKey]
      if box = App.Box.findById json.id
        box.set 'loadingPublicKey', false
        if publicKey = json.public_key
          box.set 'public_key', publicKey
        else
          box.set 'publicKeyInfo', 'Public key (~/.ssh/id_rsa.pub) not found.'

    socket.on 'aio:add_public_key_to_github', (json) ->
      if box = App.Box.findById json[App.Box.rootKey].id
        box.set 'addingPublicKeyToGitHub', false
        message = switch json.status
          when 'ok'
            'Added to GitHub!'
          when 'fail'
            switch json.error
              when 'public_key_not_found'
                box.set 'public_key', null
                'Public key (~/.ssh/id_rsa.pub) not found.'
              when 'github_validation_failed'
                'Failed to add public key to GitHub (already added?)'
              else
                "Failed to add public key to GitHub. Please try again."
        box.set 'publicKeyInfo', message

    socket.on 'aio:box_info_response', (json) ->
      json = json[App.Box.rootKey]
      if box = App.Box.findById json.id
        box.set 'memory_usage', json.memory_usage
        box.set 'storage_usage', json.storage_usage

    socket.on 'aio:fetch_user_company', (json) ->
      if currentUser = App.get('currentUser')
        currentUser.set 'hasCompany', json.r

    socket.on 'aio:fetch_closest_region', (json) ->
      region = json[App.Region.rootKey]
      App.set 'closestRegionId', region?.id

    socket.on 'aio:region_status', (regionStatus) ->
      return unless regionStatus.state? || regionStatus.region_id?
      if region = App.Region.findById regionStatus.region_id
        region.set 'state', regionStatus.state

    socket.on 'aio:parts_changed', (json) ->
      boxId = json.box_id
      if boxId == App.get('router.boxIdeController.content.id')
        # ideAutopartsController will observe changes for this property and refresh
        App.set 'partsChanged', +new Date()

  didClose: ->
    socket = @get('socket')
    socket.removeAllListeners()
