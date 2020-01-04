package _Self.buildTypes

import jetbrains.buildServer.configs.kotlin.v2019_2.*
import jetbrains.buildServer.configs.kotlin.v2019_2.buildSteps.exec
import jetbrains.buildServer.configs.kotlin.v2019_2.triggers.vcs

object Build : BuildType({
    name = "Build"

    artifactRules = "beemer => |%build.number%_beemer"
    publishArtifacts = PublishMode.SUCCESSFUL
    vcs {
        root(HttpsGithubComSimon987beemerRefsHeadsMaster)
    }
    steps {
        exec {
            name = "Build"
            path = "ci/build.sh"
            dockerImage = "golang"
        }
    }

    triggers {
        vcs {
            branchFilter = ""
        }
    }
})