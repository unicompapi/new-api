/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  AiChat01Icon,
  AiImageIcon,
  AiVideoIcon,
  Album01Icon,
  BotIcon,
  BubbleChatIcon,
  FolderManagementIcon,
  MessageMultiple01Icon,
  PackageIcon,
  Settings01Icon,
  Task01Icon,
  Video01Icon,
} from '@hugeicons/core-free-icons'
import type { IconSvgElement } from '@hugeicons/react'
import type { TFunction } from 'i18next'
import {
  UNICOMP_AI_DOWNLOAD_ACTION,
  UNICOMP_AI_EXECUTABLE,
  UNICOMP_AI_INSTALLER_NAME,
  UNICOMP_AI_VERSION,
  UNICOMP_DESKTOP_DOWNLOAD_ACTION,
  UNICOMP_DESKTOP_INSTALLER_NAME,
  UNICOMP_DESKTOP_VERSION,
} from './constants'

export type DownloadFeature = {
  icon: IconSvgElement
  title: string
  description: string
}

export type DownloadProductContent = {
  title: string
  description: string
  version: string
  installerName: string
  downloadAction: string
  localNote: string
  featureSectionTitle: string
  features: DownloadFeature[]
  requirements: string[]
  installSteps: string[]
  warning?: {
    title: string
    description: string
  }
}

export function getUniCompAiContent(t: TFunction): DownloadProductContent {
  return {
    title: t('UniComp AI Desktop Client'),
    description: t(
      'A Windows desktop companion for the UniComp API gateway. Manage API tokens, run AI chat, generate videos with Seedance 2.0, and more — all in one app.'
    ),
    version: UNICOMP_AI_VERSION,
    installerName: UNICOMP_AI_INSTALLER_NAME,
    downloadAction: UNICOMP_AI_DOWNLOAD_ACTION,
    localNote: t(
      'Local-first design with embedded SQLite — your data stays on your device'
    ),
    featureSectionTitle: t('Key Features'),
    features: [
      {
        icon: PackageIcon,
        title: t('Application Management'),
        description: t(
          'Create applications, manage API addresses, and organize access tokens securely in local SQLite storage.'
        ),
      },
      {
        icon: BotIcon,
        title: t('Model Management'),
        description: t(
          'Browse and manage available AI models connected to your API gateway.'
        ),
      },
      {
        icon: Video01Icon,
        title: t('Seedance 2.0'),
        description: t(
          'Generate AI videos with prompts, reference assets, task polling, and local generation history.'
        ),
      },
      {
        icon: BubbleChatIcon,
        title: t('AI Chat'),
        description: t(
          'Multi-turn conversations with models via the Playground interface.'
        ),
      },
      {
        icon: MessageMultiple01Icon,
        title: t('Chatroom'),
        description: t(
          'Real-time messaging with WebSocket support for team collaboration.'
        ),
      },
    ],
    requirements: [
      t('Windows 10 or later (64-bit)'),
      t('No Node.js or Python required — standalone installer'),
      t('Internet connection for API access'),
    ],
    installSteps: [
      t('Download the installer using the button above'),
      t('Run UniComp AI Setup and follow the wizard'),
      t('Launch {{app}} from the desktop or Start menu shortcut', {
        app: UNICOMP_AI_EXECUTABLE,
      }),
      t('Add your API address and token in Application Management'),
    ],
  }
}

export function getUniCompDesktopContent(t: TFunction): DownloadProductContent {
  return {
    title: t('UniComp Desktop'),
    description: t(
      'A Windows desktop workspace that brings conversations, projects, image and video workflows, tasks, works, providers, and settings into one application.'
    ),
    version: UNICOMP_DESKTOP_VERSION,
    installerName: UNICOMP_DESKTOP_INSTALLER_NAME,
    downloadAction: UNICOMP_DESKTOP_DOWNLOAD_ACTION,
    localNote: t(
      'Designed around local project organization and local application state.'
    ),
    featureSectionTitle: t('Workspace Areas'),
    features: [
      {
        icon: AiChat01Icon,
        title: t('AI Chat'),
        description: t(
          'A workspace area for conversations with configured AI services.'
        ),
      },
      {
        icon: FolderManagementIcon,
        title: t('Project Management'),
        description: t(
          'A workspace area for organizing creative work by project.'
        ),
      },
      {
        icon: AiImageIcon,
        title: t('Image Creation'),
        description: t('A workspace area for AI image creation and review.'),
      },
      {
        icon: AiVideoIcon,
        title: t('Video Creation'),
        description: t(
          'A workspace area for AI video creation and task progress.'
        ),
      },
      {
        icon: Task01Icon,
        title: t('Task Center'),
        description: t(
          'Review queued, running, completed, and failed creative tasks.'
        ),
      },
      {
        icon: Album01Icon,
        title: t('Works Library'),
        description: t(
          'Browse and organize generated images and videos from the works area.'
        ),
      },
      {
        icon: BotIcon,
        title: t('Models and Providers'),
        description: t(
          'Manage the models and service providers used by creative workflows.'
        ),
      },
      {
        icon: Settings01Icon,
        title: t('Local Settings'),
        description: t(
          'Adjust local preferences and application behavior on your device.'
        ),
      },
    ],
    requirements: [
      t('Windows 10 22H2 or later (64-bit)'),
      t('Internet connection for AI services'),
      t('Additional storage for local projects and generated media'),
    ],
    installSteps: [
      t('Download the installer using the button above'),
      t('Run {{installer}} and follow the setup wizard', {
        installer: UNICOMP_DESKTOP_INSTALLER_NAME,
      }),
      t('Launch UniComp from the desktop or Start menu shortcut'),
      t('Configure the models and service providers you plan to use'),
    ],
    warning: {
      title: t('Unsigned installer'),
      description: t(
        'This release is not code-signed. Windows SmartScreen may display a warning; verify the installer source before continuing.'
      ),
    },
  }
}
