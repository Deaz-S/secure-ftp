# Secure FTP

Client FTP/SFTP/FTPS avec interface graphique, développé en Go avec Fyne.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-green)

## Fonctionnalités

- **Protocoles supportés** : SFTP, FTPS, FTP
- **Interface graphique** : Double panneau (local/distant) style FileZilla
- **Transferts** :
  - Upload/Download avec barre de progression
  - Reprise des transferts interrompus
  - Transferts parallèles (4 par défaut)
  - File d'attente avec priorités
- **Sécurité** :
  - Vérification des clés hôtes SSH (known_hosts)
  - Alerte en cas de changement de clé (protection MITM)
  - Support TLS pour FTPS
- **Performance** :
  - Buffers optimisés (256KB - 1MB adaptatif)
  - 64 requêtes SFTP concurrentes par fichier
- **Profils de connexion** : Sauvegarde des serveurs favoris

## Capture d'écran

```
┌─────────────────────────────────────────────────────────────┐
│  Secure FTP                                           [—][×]│
├─────────────────────────────────────────────────────────────┤
│ [Connect] [Disconnect] | [Refresh] [Upload] [Download]      │
├────────────────────────┬────────────────────────────────────┤
│  Local Files           │  Remote Files                      │
│  /home/user            │  /var/www                          │
│  ├── Documents/        │  ├── html/                         │
│  ├── Downloads/        │  ├── logs/                         │
│  └── file.txt          │  └── config.php                    │
├────────────────────────┴────────────────────────────────────┤
│  Transfers: file.txt ████████░░ 80% - 1.2 MB/s              │
└─────────────────────────────────────────────────────────────┘
```

## Installation

### Prérequis

- Go 1.21 ou supérieur
- Dépendances Fyne (Linux) :
  ```bash
  # Ubuntu/Debian
  sudo apt install libgl1-mesa-dev xorg-dev

  # Fedora
  sudo dnf install mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel
  ```

### Compilation

```bash
git clone <repository>
cd secure-ftp
go build -o secure-ftp ./cmd/secureftp
```

### Exécution

```bash
./secure-ftp
```

## Configuration

Les fichiers de configuration sont stockés dans `~/.config/secure-ftp/` :

| Fichier | Description |
|---------|-------------|
| `config.json` | Configuration générale et profils |
| `known_hosts` | Clés SSH des serveurs connus |
| `logs/` | Journaux d'activité |

### Options de configuration

```json
{
  "max_parallel_transfers": 4,
  "log_level": "info",
  "theme": "system",
  "show_hidden_files": false
}
```

## Utilisation

### Connexion rapide

1. Cliquer sur **Connect**
2. Sélectionner le protocole (SFTP/FTPS/FTP)
3. Entrer les informations du serveur
4. Cliquer sur **Connect**

### Transfert de fichiers

- **Upload** : Sélectionner un fichier local → Cliquer sur **Upload**
- **Download** : Sélectionner un fichier distant → Cliquer sur **Download**
- **Glisser-déposer** : Supporter entre les panneaux

### Profils de connexion

Les profils permettent de sauvegarder les paramètres de connexion pour un accès rapide.

## Sécurité

### Vérification des clés hôtes (SFTP)

Lors de la première connexion à un serveur SFTP, une boîte de dialogue affiche l'empreinte de la clé :

```
Nouvel hôte SSH
L'authenticité de l'hôte 'serveur.com' ne peut pas être établie.
Empreinte: SHA256:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Voulez-vous faire confiance à cet hôte ?
```

Si la clé d'un serveur connu change, une alerte de sécurité s'affiche (possible attaque man-in-the-middle).

### Recommandations

- Préférer **SFTP** ou **FTPS** au FTP non sécurisé
- Vérifier les empreintes de clés lors de la première connexion
- Ne pas ignorer les alertes de changement de clé

## Architecture

```
secure-ftp/
├── cmd/secureftp/      # Point d'entrée
├── internal/
│   ├── app/            # Logique application
│   ├── config/         # Gestion configuration
│   ├── protocol/       # Clients SFTP/FTPS/FTP
│   ├── transfer/       # Gestionnaire de transferts
│   └── ui/             # Interface Fyne
├── pkg/logger/         # Système de logs
└── assets/             # Ressources (icônes)
```

## Dépendances

- [Fyne](https://fyne.io/) - Framework UI
- [pkg/sftp](https://github.com/pkg/sftp) - Client SFTP
- [jlaffaye/ftp](https://github.com/jlaffaye/ftp) - Client FTP/FTPS
- [x/crypto/ssh](https://golang.org/x/crypto/ssh) - SSH

## Licence

MIT License - Voir [LICENSE](LICENSE) pour plus de détails.
