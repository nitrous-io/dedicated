# Ruby autocomplete

Ruby autocomplete is the first major plugin for the web IDE. Using `ctags` and `ag` it is able to index a project and search the `.ruby-tags` file over standard `freighterd` ops socket functionality.


## Freighterd and opsSocket

`opSockets` (or `cops` in `freighterd`) provides a functionality for invoking commands in a shell, as part of the work for autocomplete different types of shell invocations have been added.

See [Add support for small shell](https://github.com/nitrous-io/dedicated/commit/1a6ad33bb18d3a68050e21d683381964f123fb03#diff-bd3bc2562711bb8dd029835dc4591687R123) and [Better cops `run` shell](https://github.com/nitrous-io/dedicated/pull/2155) for background.

### `minirun`

Calls `buildWithMinimalShell` which calls `su -p -c {CMD} {USER}`. `-p` preserves the environment but does not perform a full shell login procedure. This is useful in the case of `ag` when it doesn't require any profile information, it just needs to run a command as **quickly** as possible.

### `loginrun`

Calls `buildWithLoginUserShell` which calls `su --login -c {CMD} {USER}`. `{CMD}` is a combination of the command plus the user's shell with `-i -l` meaning it will be started interactively and with login invoking the full shell login procedure as if the user were logging on themselves.

This gives the command that will be invoked accessed to any configuration set in their profile RC or settings files. This is used when generating running `ctags` because it asks `bundler` what are the paths it should search for (to obtain gem paths). Because bundler might be provided as part of the `rbenv` scripts, it is necessary to evaluate this in the full context of a login.

Because Docker `su` processes usually scrub out the current environment, `loginrun` will loop through all `NITROUS_` environment variables that `freighterd` can see (thanks to Go's `os.Environ()`) and pass it into

### `run`

Calls `buildWithSuUserShell` which is similar to `buildWithLoginUserShell` but it is not started interactively giving it a speedier startup time but less profile information.

### Note about `su`

All shells run `su` because when `freighterd` accesses the container it needs to drop the elevation in privileges. `freighterd` runs on the host but needs to `exec` onto the container.

### Which one?

If there is knowledge that the command does not need any shell environment variables or login features then `minirun` is the fastest option and is great for `grep`ing files or searching files. It is then best left to the implementer to test if `run` or `loginrun` are the best options: where `run` will be faster than `loginrun` (due to interactive).

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
