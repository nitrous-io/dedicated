# Ruby autocomplete

Ruby autocomplete is the first major plugin for the web IDE. Using `ctags` and `ag` it is able to index a project and search the `.ruby-tags` file over standard `freighterd` ops socket functionality.

## App.CodePluginsManager

`CodePluginsManager` handles the following tasks

* registering plugin and global plugins
* initializing plugins for a given type (when requested by `Document`)
* tracking plugins so they can be located by their global plugin
* tracking global plugin so they can be removed when the last plugin is closed (global plugin no longer needed)
* tracking the options available to the IDE (`container`, `opsSocket`, `userSettings`) so they can be passed to the plugins

## PluginBase

`PluginBase` provides the following functions:

* `exec`: which uses ops socket to run commands (in a lightweight form or with a full login shell)
* `parseCTags`: taking data and a callback, a `ctags` format file can be looped over
* `searchCTags`: making use of `exec` with a `minimal` shell, `/usr/bin/ag` is invoked to search a `ctags` format file for a specified term

When extending from a `PluginBase` a plugin should overload the function `cTagsLocation` to indicate where the `ctags` format file should be found. This is because ctags file differ per plugin type (each have custom definitions and filter specific files).

## GlobalRuby

`GlobalRuby` is a global plugin that manages the indexing of a project by generating the `.ruby-tags` file. Because each document plugin runs separately they need to tell their plugin to perform an update of the ctags file after a document save, otherwise we would have every single document plugin updating the ctags file.

Instead these plugins (class `Ruby`) ask for their parent (`@globalPlugin` populated at init by `App.CodePluginsManager`) and then call `triggerCtagsUpdate` on it. `GlobalRuby` uses a short interval timer to coalesce multiple requests at the same time.

When a `GlobalRuby` is initialized for the first time (as part of the first document plugin being initialized) it will ensure that the required dependencies are installed. Once the dependencies are installed (using `apt-get`) then `softwareRequirements` is set as a container metadata to prevent this process happening each tab open.

## Ruby

`Ruby` is a document plugin (meaning each document tab has its own instance), it has access to the CodeMirror instance and responds to the `autocomplete` function called by `Editor` on a `keyup` CodeMirror event.
