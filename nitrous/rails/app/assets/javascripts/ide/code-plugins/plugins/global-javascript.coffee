#= require ./global-plugin-base

klass = class GlobalJavascript extends GlobalPluginBase
  TYPE = 'javascript'

  # taken from js-ctags.conf to Base64
  CTAG_EXTENSIONS = 'LS1sYW5nZGVmPWpzDQotLWxhbmdtYXA9anM6LmpzDQotLXJlZ2V4LWpzPS8oLHwoO3xeKVsgXHRdKih2YXJ8bGV0fChbQS1aYS16XyRdW0EtWmEtejAtOV8kLl0rXC4pKikpWyBcdF0qKFtBLVphLXowLTlfJF0rKVsgXHRdKj1bIFx0XSpcey9cNS8sb2JqZWN0Lw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSo9WyBcdF0qZnVuY3Rpb25bIFx0XSpcKC9cNS9mLGZ1bmN0aW9uLw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSo9WyBcdF0qXFsvXDUvLGFycmF5Lw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSo9WyBcdF0qW14iXSdbXiddKi9cNS8sc3RyaW5nLw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSo9WyBcdF0qKHRydWV8ZmFsc2UpL1w1Lyxib29sZWFuLw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSo9WyBcdF0qWzAtOV0rL1w1LyxudW1iZXIvDQotLXJlZ2V4LWpzPS8oLHwoO3xeKVsgXHRdKih2YXJ8bGV0fChbQS1aYS16XyRdW0EtWmEtejAtOV8kLl0rXC4pKikpWyBcdF0qKFtBLVphLXowLTlfJF0rKVsgXHRdKj1bIFx0XSouKyhbLDs9XXwkKS9cNS92LHZhcmlhYmxlLw0KLS1yZWdleC1qcz0vKCx8KDt8XilbIFx0XSoodmFyfGxldHwoW0EtWmEtel8kXVtBLVphLXowLTlfJC5dK1wuKSopKVsgXHRdKihbQS1aYS16MC05XyRdKylbIFx0XSpbIFx0XSooWyw7XXwkKS9cNS92LHZhcmlhYmxlLw0KLS1yZWdleC1qcz0vZnVuY3Rpb25bIFx0XSsoW0EtWmEtejAtOV8kXSspWyBcdF0qXChbXildKlwpL1wxL2YsZnVuY3Rpb24vDQoNCi0tcmVnZXgtanM9LygsfF4pWyBcdF0qKFtBLVphLXpfJF1bQS1aYS16MC05XyRdKylbIFx0XSo6WyBcdF0qXHsvXDIvcCxvYmplY3QvDQotLXJlZ2V4LWpzPS8oLHxeKVsgXHRdKihbQS1aYS16XyRdW0EtWmEtejAtOV8kXSspWyBcdF0qOlsgXHRdKmZ1bmN0aW9uWyBcdF0qXCgvXDIvZixmdW5jdGlvbi8NCi0tcmVnZXgtanM9LygsfF4pWyBcdF0qKFtBLVphLXpfJF1bQS1aYS16MC05XyRdKylbIFx0XSo6WyBcdF0qXFsvXDIvcCxhcnJheS8NCi0tcmVnZXgtanM9LygsfF4pWyBcdF0qKFtBLVphLXpfJF1bQS1aYS16MC05XyRdKylbIFx0XSo6WyBcdF0qW14iXSdbXiddKi9cMi9wLHN0cmluZy8NCi0tcmVnZXgtanM9LygsfF4pWyBcdF0qKFtBLVphLXpfJF1bQS1aYS16MC05XyRdKylbIFx0XSo6WyBcdF0qKHRydWV8ZmFsc2UpL1wyL3AsYm9vbGVhbi8NCi0tcmVnZXgtanM9LygsfF4pWyBcdF0qKFtBLVphLXpfJF1bQS1aYS16MC05XyRdKylbIFx0XSo6WyBcdF0qWzAtOV0rL1wyL3AsbnVtYmVyLw0KLS1yZWdleC1qcz0vKCx8XilbIFx0XSooW0EtWmEtel8kXVtBLVphLXowLTlfJF0rKVsgXHRdKjpbIFx0XSpbXj1dKyhbLDtdfCQpL1wyL3AsdmFyaWFibGUv'
  CMD = "ctags --options=!CONF! -f .js-tags -R --languages=js --exclude=dist --exclude=.git --exclude=log --exclude=tmp ."

  CTAGS_FILE_VER = "1"

  triggerCtagsUpdate: (parent)->
    return unless @opsSocket

    return if @activeTimers[parent]

    @activeTimers[parent] = window.setTimeout =>
      if @container?.get('metadata.ctagsJsConfigFile') != CTAGS_FILE_VER
        return @createCtagsConfigFile(parent)

      @runCtags(parent)
    , 50

  createCtagsConfigFile: (parent)->
    @opsSocket.writeFile @getCtagsConfPath(), atob(CTAG_EXTENSIONS), (err, absolutePath)=>
      if err
        console.log "Error invoking ctags config file: ", absolutePath, err
        return

      @container?.setMetadata(ctagsJsConfigFile: CTAGS_FILE_VER)
      @runCtags(parent)

  runCtags: (parent)->
    parsedCmd = CMD.replace("!CONF!", @getCtagsConfPath())
    cmd = [
      "cd #{parent} && #{parsedCmd}"
    ]
    console.log "Invoking #{cmd}"
    @opsSocket.loginrun cmd, parent, [], (err, cmdErr, stdout, stderr) =>
      @activeTimers[parent] = null

      if cmdErr || err
        if stderr && (stderr.indexOf('cannot open option file') != -1)
          console.log("User deleted option file, will recreate")
          @createCtagsConfigFile(parent)
          return
        else if stderr && (stderr.indexOf('doesn\'t look like a tag file; I refuse to overwrite it.') != -1)
          @opsSocket.deleteFile "#{parent}/.js-tags", (err, absolutePath)=>
            if err
              console.log "Error deleting js-tags: ", absolutePath, err
              return

            @runCtags(parent)

        console.log "Error invoking ctags update:", err, cmdErr, stderr, stdout
        return

      for plugin in (App.CodePluginsManager.activePlugins['javascript'] || [])
        plugin.ctagsUpdated()

  getCtagsConfPath: ->
    return "#{@getHomePath()}/.ctags-js.conf"

App.CodePluginsManager.registerGlobalPlugin "javascript", klass