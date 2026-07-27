# Federation Todo App 🚀

Proyek referensi ini mendemonstrasikan implementasi **GraphQL Federation (Apollo Federation v2)** menggunakan **Go**, **gqlgen**, **PostgreSQL**, dan **Docker Compose**. Proyek ini dirancang sebagai studi kasus nyata mengenai bagaimana memecah aplikasi monolitik menjadi subgraf-subgraf (subgraphs) terfederasi yang modular dan terisolasi secara database.

---

## Daftar Isi
1. [Arsitektur Sistem](#arsitektur-sistem)
2. [Mengenal GraphQL Federation](#mengenal-graphql-federation)
3. [Alur Eksekusi Query Lintas Subgraf (Query Flow)](#alur-eksekusi-query-lintas-subgraf-query-flow)
4. [Mengapa Memilih Go + gqlgen](#mengapa-memilih-go--gqlgen)
5. [Struktur Proyek](#struktur-proyek)
6. [Langkah Menjalankan Aplikasi](#langkah-menjalankan-aplikasi)
7. [Panduan Pengembangan Lokal](#panduan-pengembangan-lokal)
8. [Catatan Penting & Troubleshooting (Pelajaran dari Proyek Ini)](#catatan-penting--troubleshooting-pelajaran-dari-proyek-ini)
9. [Rencana Peningkatan Produksi (Production Hardening)](#rencana-peningkatan-produksi-production-hardening)

---

## Arsitektur Sistem

Aplikasi ini dibagi menjadi beberapa komponen independen yang diorkestrasi menggunakan Docker Compose:

```
                     ┌─────────────────────────┐
   Client  ────────▶ │      Apollo Router      │ :4000 (Supergraph Gateway)
                     └────────────┬────────────┘
                     ┌────────────┴────────────┐
                     ▼                         ▼
         ┌───────────────────────┐ ┌───────────────────────┐
         │     users-service     │ │     todos-service     │
         │     (Go + gqlgen)     │ │     (Go + gqlgen)     │
         │     Port: 4001        │ │     Port: 4002        │
         └───────────┬───────────┘ └───────────┬───────────┘
                     ▼                         ▼
                Database:                 Database:
                users_db (Postgres)       todos_db (Postgres)
```

1. **Apollo Router (Gateway)**: Pintu masuk utama untuk klien. Menerima query GraphQL tunggal, menganalisis Query Plan, membagi dan menyebarkan query ke subgraf yang relevan, lalu menggabungkan hasilnya kembali ke klien.
2. **users-service**: Subgraf yang bertanggung jawab atas entitas `User`. Memiliki database sendiri (`users_db`).
3. **todos-service**: Subgraf yang bertanggung jawab atas entitas `Todo`. Subgraf ini memperluas (extends) entitas `User` untuk menambahkan daftar todo tanpa perlu berinteraksi langsung dengan database pengguna (`users_db`).
4. **PostgreSQL**: Dua database terisolasi untuk memastikan prinsip *microservices* benar-benar terjaga.

---

## Mengenal GraphQL Federation

GraphQL Federation adalah arsitektur yang memungkinkan Anda untuk membagi satu skema GraphQL besar (Supergraph) menjadi bagian-bagian skema yang lebih kecil (Subgraphs) yang dikembangkan dan dideploy oleh tim yang berbeda secara independen.

### Konsep Kunci Federation Lintas Subgraf

#### 1. Direktif `@key` (Kunci Entitas)
Direktif `@key` digunakan untuk menentukan field mana yang menjadi pengidentifikasi unik (primary key) dari suatu entitas sehingga subgraf lain dapat mereferensikannya atau memperluasnya.
Di proyek ini:
*   Di **users-service**, entitas `User` didefinisikan dengan `@key(fields: "id")`:
    ```graphql
    type User @key(fields: "id") {
      id: ID!
      username: String!
      email: String!
    }
    ```

#### 2. Entitas Lintas Batas (Entity Extension)
Sebuah subgraf dapat memperluas entitas yang dimiliki oleh subgraf lain. 
Di proyek ini, **todos-service** ingin menambahkan field `todos` ke dalam tipe `User` yang dimiliki oleh **users-service**.
Di dalam `todos-service/schema.graphqls`:
```graphql
type User @key(fields: "id") {
  id: ID!
  todos: [Todo!]!
}
```
*Catatan: `todos-service` tidak perlu tahu detail nama lengkap (`fullName`) atau email dari `User`. Ia hanya perlu mendeklarasikan `id` sebagai kunci referensi untuk menempelkan data `todos`.*

#### 3. Resolusi Entitas (Entity Resolution)
Ketika subgraf memperluas sebuah entitas, ia harus menyediakan resolver khusus untuk membangun entitas tersebut dari data kunci yang dikirim oleh Router.
Di Go (gqlgen), ini dilakukan melalui resolver entitas di berkas [resolver.go](file:///Users/sammidev/Downloads/federation-todo-app/todos-service/resolver.go):
```go
func (e *entityResolver) FindUserByID(ctx context.Context, id string) (*User, error) {
    return &User{ID: id}, nil
}
```
Setelah representasi dasar `User` dikembalikan, gqlgen akan memanggil resolver field untuk mengisi field terfederasi:
```go
func (u *userResolver) Todos(ctx context.Context, obj *User) ([]*Todo, error) {
    // Mengambil data todos berdasarkan obj.ID (User ID) dari database todos_db
}
```

---

## Alur Eksekusi Query Lintas Subgraf (Query Flow)

Bagaimana Apollo Router memproses query federasi? Mari kita telusuri alur eksekusi untuk query berikut:

```graphql
query GetUsersWithTodos {
  users {
    id
    username
    todos {
      id
      title
    }
  }
}
```

### Jalur Komunikasi Data

```
 Client             Apollo Router         users-service        todos-service
   │                      │                     │                    │
   │ ─── 1. Kirim Query ─▶│                     │                    │
   │                      │ ── 2. Query Users ─▶│                    │
   │                      │ ◀── 3. Data Users ──│                    │
   │                      │                                          │
   │                      │ ────── 4. Kirim Representasi User ──────▶│
   │                      │ ◀───── 5. Kembalikan Data Todos ─────────│
   │ ◀── 6. Gabung Data ──│                                          │
```

### Penjelasan Langkah demi Langkah:
1.  **Client** mengirimkan query GraphQL ke **Apollo Router** di port `:4000`.
2.  **Apollo Router** menganalisis skema dan menyadari bahwa:
    *   Query `users` dan field `username` hanya diketahui oleh **users-service**.
    *   Field `todos` pada objek `User` didefinisikan oleh **todos-service**.
3.  **Router** mengirimkan request HTTP POST pertama ke **users-service** (`http://users-service:4001/query`) meminta field `id` dan `username`.
4.  **users-service** mengambil data dari `users_db` dan mengembalikan JSON:
    ```json
    {
      "data": {
        "users": [
          { "id": "user-uuid-111", "username": "sammidev" }
        ]
      }
    }
    ```
5.  **Router** kemudian menyusun request federasi untuk mengambil field `todos`. Router mengirimkan request ke **todos-service** (`http://todos-service:4002/query`) menggunakan query internal `_entities` dengan representasi kunci dari langkah sebelumnya:
    ```json
    {
      "query": "query($representations:[_Any!]!){_entities(representations:$representations){...on User{todos{id title}}}}",
      "variables": {
        "representations": [
          { "__typename": "User", "id": "user-uuid-111" }
        ]
      }
    }
    ```
6.  Di **todos-service**:
    *   Fungsi `FindUserByID` dipanggil dengan parameter `id: "user-uuid-111"` untuk membuat objek instansiasi `User`.
    *   Fungsi `userResolver.Todos` dipanggil dengan objek `User` tersebut untuk menanyakan todos miliknya ke database `todos_db`.
    *   Hasilnya dikembalikan ke Router.
7.  **Apollo Router** menggabungkan (stitches) data pengguna dari **users-service** dan data todo dari **todos-service** menjadi satu kesatuan objek JSON, lalu mengirimkannya kembali ke klien.

---

## Mengapa Memilih Go + gqlgen

[gqlgen](https://github.com/99designs/gqlgen) is schema-first GraphQL library terpopuler di komunitas Go.
*   **Aman secara Tipe (Type-safe)**: gqlgen menghasilkan struct Go berdasarkan skema GraphQL Anda, meminimalkan bug runtime karena kesalahan tipe data.
*   **Performa Tinggi**: Ditulis dalam Go, sangat efisien dalam penggunaan memori dan CPU, cocok untuk microservices GraphQL yang membutuhkan latensi rendah.
*   **Dukungan Penuh Federation v2**: Dilengkapi plugin bawaan untuk menghasilkan metadata federasi GraphQL secara otomatis.

---

## Struktur Proyek

Proyek ini menggunakan struktur direktori yang datar (flat layout) per layanan untuk menyederhanakan pemahaman:

```
federation-todo-app/
├── postgres/                 # Docker config untuk inisialisasi Postgres
├── router/                   # Konfigurasi Apollo Router (router.yaml)
├── supergraph-config.yaml    # Konfigurasi penyusunan supergraph oleh Rover CLI
├── docker-compose.yml        # Orkestrasi Docker container
├── users-service/            # Layanan pengguna (subgraf 1)
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── gqlgen.yml            # Konfigurasi generator gqlgen
│   ├── main.go               # Inisialisasi HTTP server & database pool
│   ├── resolver.go           # Implementasi resolver logika bisnis (tulisan tangan)
│   ├── schema.graphqls       # Definisi skema GraphQL (SDL)
│   ├── schema.sql            # Migrasi database otomatis saat booting
│   └── tools.go              # Dependensi generator agar tidak dibersihkan oleh go mod tidy
└── todos-service/            # Layanan todo (subgraf 2)
    ├── (struktur yang sama seperti users-service)
```

---

## Langkah Menjalankan Aplikasi

Pastikan Anda memiliki **Docker** dan **Docker Compose** yang sudah berjalan di sistem Anda.

### 1. Menjalankan Server
Jalankan perintah berikut di root folder proyek:
```bash
docker compose up --build
```

Proses orkestrasi akan berjalan otomatis:
*   Membangun citra (image) Docker untuk `users-service` dan `todos-service`.
*   Menjalankan container PostgreSQL dan menjalankan skrip migrasi `.sql` di setiap layanan.
*   Container `composer` (one-shot) akan menunggu hingga kedua subgraf sehat di port `:4001` and `:4002`, lalu mengeksekusi `rover supergraph compose` untuk memproduksi skema gabungan `supergraph.graphql`.
*   **Apollo Router** membaca `supergraph.graphql` dan mulai melayani request di port `:4000`.

### 2. Mengakses Playground & Sandbox
*   **Apollo Sandbox (Gateway Utama)**: Buka [http://localhost:4000](http://localhost:4000) di peramban Anda. Di sini Anda bisa mengeksekusi query federasi lintas layanan.
*   **users-service Playground (Bypass/Debug)**: [http://localhost:4001](http://localhost:4001)
*   **todos-service Playground (Bypass/Debug)**: [http://localhost:4002](http://localhost:4002)

### 3. Contoh Query Lintas Subgraf untuk Dicoba di Port 4000

#### A. Membuat Pengguna Baru (Mutation)
```graphql
mutation CreateUser {
  createUser(input: { username: "sammidev4", email: "sam@example.com", fullName: "Sam Dev" }) {
    id
    username
    fullName
  }
}
```

#### B. Membuat Todo untuk Pengguna Tersebut (Mutation)
*Ganti `userID` dengan ID hasil mutasi di atas:*
```graphql
mutation CreateTodo($userID: ID!) {
  createTodo(input: { title: "Belajar GraphQL Federation", userID: $userID, description: "Memahami cara kerja Apollo Router dan subgraphs" }) {
    id
    title
    completed
    user {
      id
      username
    }
  }
}
```

#### C. Mengambil Semua Pengguna beserta Todonya (Federated Query)
```graphql
query GetUsersAndTodos {
  users {
    id
    username
    fullName
    todos {
      id
      title
      completed
      createdAt
    }
  }
}
```

---

## Panduan Pengembangan Lokal

Jika Anda ingin menjalankan layanan secara lokal tanpa Docker untuk mempercepat siklus debug:

### Prasyarat
*   Go versi 1.23+ terinstal di mesin Anda.
*   Database PostgreSQL lokal yang menyala dengan kredensial `postgres:postgres` dan dua database bernama `users_db` dan `todos_db`.

### Langkah Eksekusi (Contoh untuk `todos-service`)
```bash
cd todos-service
# Sinkronisasi dependensi modul
go mod tidy
# Generate kode GraphQL (jika ada perubahan skema)
go generate ./...
# Jalankan aplikasi dengan environment variable penunjuk database
DATABASE_URL=postgres://postgres:postgres@localhost:5432/todos_db?sslmode=disable go run .
```

---

## Catatan Penting & Troubleshooting (Pelajaran dari Proyek Ini)

Selama pengembangan proyek ini, terdapat beberapa kendala konfigurasi yang berhasil diidentifikasi dan diselesaikan. Berikut adalah ringkasannya sebagai referensi jika Anda menemui masalah serupa:

### 1. Masalah Dependensi Tool Generator (`go.sum`)
**Masalah**: Menjalankan `go generate ./...` di dalam Dockerfile gagal dengan error:
```
missing go.sum entry for module providing package golang.org/x/tools/go/packages
```
**Penyebab**: `go mod tidy` menghapus pustaka generator yang tidak langsung diimpor dalam kode aplikasi aktif. Karena `gqlgen` dipanggil via `go run github.com/99designs/gqlgen generate`, dependensinya tidak tercatat dalam skema impor standar.
**Solusi**: Dibuat file [tools.go](file:///Users/sammidev/Downloads/federation-todo-app/todos-service/tools.go) di setiap layanan dengan build tag `//go:build tools` untuk secara eksplisit mengimpor pustaka generator utama. Ini memaksa Go mencatat dependensi tersebut di `go.mod` dan `go.sum` sehingga aman dari pembersihan oleh `go mod tidy`.

### 2. Konflik File Resolver (`resolver_gen.go` vs `resolver.go`)
**Masalah**: Terjadi error kompilasi karena duplikasi deklarasi tipe data `Resolver` dan resolver method-nya:
```
./resolver_gen.go:9:6: Resolver redeclared in this block
```
**Penyebab**: Secara default, `gqlgen.yml` diatur untuk mengeluarkan kode resolver ke berkas baru bernama `resolver_gen.go`. Karena seluruh resolver sesungguhnya telah diimplementasikan di `resolver.go`, generator kebingungan dan melahirkan file duplikat yang bertabrakan saat kompilasi.
**Solusi**: Mengubah konfigurasi target resolver di berkas `gqlgen.yml` menjadi `resolver.go`. Dengan begini, generator akan langsung menggabungkan (merge) kode baru ke dalam berkas `resolver.go` jika terdapat perubahan skema GraphQL tanpa membuat file baru yang bentrok.

### 3. Masalah Tipe Data Slice Pointer (`omit_slice_element_pointers`)
**Masalah**: Kesalahan tanda tangan metode (wrong method signature) pada resolver list/slice:
```
cannot use &queryResolver{...} as QueryResolver value: wrong type for method Todos (have []*Todo, want []Todo)
```
**Penyebab**: Berkas `gqlgen.yml` memiliki konfigurasi `omit_slice_element_pointers: true`. Ini menginstruksikan generator untuk menghasilkan tipe data slice berupa nilai mentah (`[]Todo`), sementara kode resolver kita yang berinteraksi dengan database mengembalikan slice pointer (`[]*Todo`).
**Solusi**: Mengubah konfigurasi `omit_slice_element_pointers` menjadi `false` di `gqlgen.yml` untuk menyelaraskan skema kode generator dengan implementasi resolver aktual yang berbasis pointer.

---

## Rencana Peningkatan Produksi (Production Hardening)

Jika Anda ingin membawa proyek ini ke lingkungan produksi, pertimbangkan untuk menerapkan peningkatan berikut:

1.  **Proteksi Masalah N+1 (DataLoader)**:
    Jika data pengguna dan todo bertambah besar, pemanggilan entity resolver lintas subgraph untuk daftar panjang dapat memicu masalah query N+1 database. Gunakan pustaka DataLoader seperti [graph-gophers/dataloader](https://github.com/graph-gophers/dataloader) atau [vikstee/go-dataloader](https://github.com/vikstee/go-dataloader) untuk melakukan batching query SQL.
2.  **Manajemen Migrasi Database**:
    Ganti eksekusi otomatis `schema.sql` saat booting dengan tools migrasi versioned seperti `golang-migrate` atau `dbmate` agar skema database dapat dilacak dan di-rollback dengan aman.
3.  **Apollo Router Hardening**:
    *   Aktifkan **APQ (Automatic Persisted Queries)** di file konfigurasi router untuk menghemat bandwidth antara client dan gateway.
    *   Tambahkan konfigurasi *rate-limiting*, *CORS*, dan autentikasi token JWT di level Router (`router.yaml`) agar subgraph tidak terpapar langsung ke publik tanpa filter keamanan.
4.  **Skema Registry**:
    Alih-alih menyusun skema supergraph menggunakan container `composer` secara manual saat booting, gunakan skema registry terpusat (seperti Apollo GraphOS atau Hive) untuk memvalidasi skema setiap kali ada perubahan sebelum dideploy ke Router.
