マイクロサービス構成を作る上で、gRPC でアプリケーションを繋ぎ、それらを GKE+kubernetes で動かすというのは有力な選択肢の一つだと思います。ここでは実際にこの構成で動くAPIを作る手順を書いてみます。作ったコードと kubernetes の設定ファイルは以下のリポジトリに置いてあります。必要に応じて参照して下さい。

- https://github.com/gong023/micro-sample

なお、gRPC, GKE といったものの概要説明・メリット説明などはなるべく省きます。これらは公式ドキュメントで説明されているため、そちらを見るのが良いです。このエントリの末尾にリンクをまとめて貼っておきます。

## 作る構成

インフラ部分は GKE+kubernetes で、その上に gRPC でやり取りするアプリケーション群が動くという形でAPIを作っていきます。

アプリケーション部分は frontend と backend に分け、各々マイクロサービスのように振る舞います。frontend は外部のネットワークからの http リクエストを受けて裏側のサービスに gRPC でリクエストする役割を持ちます。backend はこの gRPC に応答を返し、外部のネットワークには公開しません。言語は go を使います。

```
   internet
    ↓  ↑ (http)
┏---↓--↑---------┐
|   ↓  ↑         |
|  frontend      |
|   ↓ ↑ (gRPC)   |
|  backend       |
|                |
└-GKE+kubernetes-┘
```

## gRPCの説明

元も子もない説明ですが、gRPC というのは google が作った RPC です。つまりコミュニケーションの決め事（とその実装）です。REST や GraphQL といったものと比較して語られることもありますが、それらより強くマイクロサービス間でのやり取りに使われることを意識している印象を受けます。

gRPC は基本的には http2 + protocol buffers で動きます。なので簡単に言って早いです。一つの接続を保持しつつ複数のリクエストのデータを並列で処理することができ、ブロッキングが少ないです（http2 の機能）。またリクエスト・レスポンスのデータは全てバイナリでやり取りでき、 json string をパースするような手間が要らないので CPU に優しいです（protocol buffers の機能）。

速度面以外のメリットもあります。リクエスト・レスポンスに型付けを行えるという点です。通常 gRPC でやり取りされるリクエスト・レスポンスは `.proto` ファイルに書いた定義に従う事になります。この `.proto` 定義は様々な言語に翻訳でき、リクエスト元とリクエスト先で共有できるので双方でどのような型をやり取りするのか明確にしておくことができます。

## proto定義を作る

実際に gRPC を使ったアプリケーションを作っていきます。とりあえず `.proto` ファイルを作って今回作るアプリケーションの定義をしましょう

```
// calc.proto

syntax = "proto3";

service Calc {
    rpc Increment(NumRequest) returns (NumResponse) {}
}

message NumRequest {
    int64 val = 1;
}

message NumResponse {
    int64 val = 1;
}
```

`service` で定義されているのが今回作るアプリケーションです。`Calc` という名前で、受け取った int を+1して返す `Increment` という機能を持つものとします。

`message` で定義されているのはそれぞれリクエストとレスポンスで使う型です。今回は int の値が一つあれば十分なので、フィールドには `int64 val` のみを宣言しておきます。

これらの定義に従ってリクエストとレスポンスの型、そして service の interface になるコードを生成します。

生成には `protoc` というコマンドを使うため、必要であれば以下からインストールして下さい。

- https://developers.google.com/protocol-buffers/docs/downloads

また今回は go のコードを生成するため、それ用のプラグインも入れておきます。

```bash
go get -u github.com/golang/protobuf/protoc-gen-go
```

calc.proto を置いたところに `gen` というディレクトリを作り、以下のコマンドを実行すると `calc.pb.go` というファイルが得られるはずです。

```bash
protoc --go_out=plugins=grpc:gen calc.proto
```

この `calc.pb.go` はこれから作る frontend, service のアプリケーションで使うので GOPATH で見える位置に置いて下さい。

なお `calc.pb.go` の中身を見てみると、`type Calc interface`, `type NumRequest struct`, `type NumResponse struct` といったコードが見つけられると思います。これらが protoc で自動生成されたコードです。今回は go しか使わないのでありがたみが薄いかもしれませんが、protoc は一つの定義から様々な言語でコードを生成できるため、リクエスト元とリクエスト先の言語が違っても問題ありません。対応言語は2017年9月現在 C++, Java, Python, Go, Ruby, C#, Node.js, Android Java, Objective-C, PHP のようです。

## frontendを作る

proto を作ったので実際に frontend アプリケーションの実装をしていきます。最初に書いたとおり、今回 frontend というのは「外部のネットワークからの http リクエストを受けて裏側のサービスに gRPC でリクエストする役割」を持たせます。

[今回はこんな感じで書きました。](https://github.com/gong023/micro-sample/blob/master/src/frontend/main.go)（色々雑なのは許してください）

ピックアップすると以下の辺りが重要になります。

```go
    conn, err := grpc.Dial(servName+":8000", grpc.WithInsecure(), grpc.WithUnaryInterceptor(
        grpc_zap.UnaryClientInterceptor(logger),
    ))

    // ...

    client := pb.NewCalcClient(conn)
    ctx := context.Background()
    res, err := client.Increment(ctx, &pb.NumRequest{Val: int64(val)})

    // ...
}
```

`grpc.Dial` で backend との接続を確立します。それを `pb.NewCalcClient` に渡してあげると Increment が定義されている `Calc interface` を得ることができます。Increment は `pb.NumRequest` を引数として取り、`pb.NumResponse` を返します。実際にやってみるとよりわかりやすいですが、楽に proto 定義に従った実装ができると思います。

なお今回 `grpc.WithUnaryInterceptor` を使っていますが、これは必須ではないです。UnaryInterceptor というのは一般的な gRPC コールの middleware です。それに [grpc_zap](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/master/logging/zap) を渡してロギングしやすくしています。

## backendを作る

まだ backend がないので、先の frontend のコードを実行して `curl "http://localhost:8080/increment?val=1"` のようにしてもエラーになるだけだと思います。frontend からの接続を受け、応答を返す backend を実装してあげます。

[こんな感じで実装しました](https://github.com/gong023/micro-sample/blob/master/src/backend/main.go)

重要なのは main 内に書かれている以下と

```go
        lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

        // ...

        server := grpc.NewServer(grpc.UnaryInterceptor(
                grpc_zap.UnaryServerInterceptor(logger),
        ))

        pb.RegisterCalcServer(server, &CalcService{})
        server.Serve(lis)
```

`CalcService struct` です。

```go
type CalcService struct{}

func (s *CalcService) Increment(ctx context.Context, req *pb.NumRequest) (*pb.NumResponse, error) {
        req.Val++
        return &pb.NumResponse{Val: req.Val}, nil
}
```

`CalcService` には proto で生成された interface を実装させます。NumRequest を受けて NumResponse を返さなければなりません。

main 内のコードで実際に listen を始めますが、`RegisterCalcServer` は引数に proto の interface を実装した値が必要になります。ここでリクエスト元とリクエスト先で proto に定義された約束が守られているのがわかると思います。`UnaryInterceptor` は今回も必須ではありません。

これで frontend と backend が出揃ったので、各々実行し `curl "http://localhost:8080/increment?val=1"` 等するとうまく結果が得られるはずです。サンプルだと json を返すようにしています。

余談ですが、ここまでのコードで gRPC は別のマシンに実装された関数を呼ぶような感覚で使えることがわかると思います。REST でインターフェースを決めていこうとするとパスの位置や http メソッドの種類で揉めることがありますが、gRPC のこの形式だとそういった事が起こりづらいのを利点に挙げる人もいます。いいインターフェースを作ることは大切ですが、どうしても REST でやろうとすると複数の正解があったりして、そういったものを延々議論するのは生産性が高いとも思えないので個人的にもこれは賛成です。

## GKE+kubernetesの説明

まず kubernetes というのは docker を始めとした container 技術をオーケストレーションするためのツールです。ここで言うオーケストレーションというのは、例えば container が停止したら復活させるとか、リクエストが増えたら container 数を増やすとかいった操作です。kubernetes はアプリケーションは全て container で扱うことを前提としているため、そういった操作を柔軟かつ簡単に行うことができるようになっています。

GKE は Google Cloud Platform のいち機能で、主に kubernetes の機能をユーザーに提供します。それだけなら kubernetes を直接使えばいいように見えますが、実のところ kubernetes という仕組み自体がマイクロサービスのようになっており、GKE はその構成の立ち上げを簡単にやってくれます。

kubernetes は master node と node という二つの部分からなります。node が実際にアプリケーションの container がホスティングされるところで、master node がその node 内の操作を行うAPIを提供します。ユーザーは master node が公開したAPIを使ってCLI,GUI問わず container の操作を行うことができます。

## docker imageの準備

先ほど作った frontend, backend を kubernetes で扱えるようにするために dockerize していくのですが、ここからはGCPプロジェクトが必要になるのでなければ作って下さい。また gcloud, kubectl といったコマンドも使うのでなければ以下を参考にインストールとセットアップをして下さい。

- https://cloud.google.com/sdk/downloads

GCPの準備が整ったら docker image を作成します。今回は [こちら](https://github.com/gong023/micro-sample/blob/master/src/frontend/Dockerfile) と [こちら](https://github.com/gong023/micro-sample/blob/master/src/backend/Dockerfile) のような Dockerfile を作りました。

image の作成は以下のように普通に `docker build` を使えば良いですが、名前規則は `gcr.io/$PROJECT_ID/$NAME` としましょう。$NAME は任意ですが $PROJECT_ID は自分のGCPプロジェクトの id です。この名前規則でないと後述の Container Registry に push できません（確か）。

```bash
docker build -t gcr.io/$PROJECT_ID/micro-sample-frontend:v0.1 .
docker build -t gcr.io/$PROJECT_ID/micro-sample-backend:v0.1 .
```

docker image ができたら Google Container Registry に push します。Google Container Registry というのはデフォルトで非公開になっている [Docker Hub](https://hub.docker.com/) みたいなやつです。

```bash
gcloud docker -- push gcr.io/$PROJECT_ID/micro-sample-frontend:v0.1
gcloud docker -- push gcr.io/$PROJECT_IDt/micro-sample-backend:v0.1
```

push できたら [GCP console](https://console.cloud.google.com) の左上メニュー > Container Registry から確認できるはずです。ここに push された docker image を kubernetes で使っていきます。

## container clusterの作成

先述したとおり kubernetes という仕組み自体一つのマイクロサービスのようなものなので、まずそれを立ち上げます。立ち上げると言っても簡単で以下のコマンドを実行するだけです。

```bash
gcloud container clusters create micro-sample --num-nodes=2
```

実行には数分かかると思われます。以下のようなコマンドで cluster の状態を確認できます。

```bash
gcloud container clusters list
gcloud container clusters describe $CLUSTER_NAME
```

自分の cluster 上の kubernetes のバージョンは master node, node ともに1.7.5でした。ちなみにですが GCP console > Container Engine > Container Cluster から kubernetes のバージョンアップなどもできたりするので覚えておくと便利です。

## kubernetesの設定について

ここからは実際に kubernetes で使う設定ファイルを yaml で書いて実行していきます。設定についてはこちらに色々まとまっているのですが、バラエティが豊富でとっつきづらい部分があると思います。

- https://kubernetes.io/docs/concepts/

正直言って自分もまだちゃんと全部読めていないのですが、取っ掛かりとしては [Pod](https://kubernetes.io/docs/concepts/workloads/pods/pod-overview/), [Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/), [Service](https://kubernetes.io/docs/concepts/services-networking/service/) を押さえれば十分ではないかなと思っています。これらを基本として派生した概念が多い印象を受けます。

まず Pod は container の別名みたいなものです。Pod の制御は常に kubernetes が行い、ユーザーが何か直接変更を加えたりすることはありませんが、基本単位として把握しておく必要があります。

Deployment は Pod の集合体の定義です。配布はどの image を使うのか、また RollingUpdate か Blue/Green かといった文字通りデプロイの設定をするような項目もあるのですが、それ以外にもいくつの Pod が常に立ち上がっていればよいのか、もしくはオートスケールにして最低いくつ最高いくつの Pod を持てるのか、Pod を増やす閾値は何かといったことまで定義できます。個人的には色々できすぎて割とカオスな印象を持っていますが、今後の流れとしては Deployment から分離して別の概念として取り扱っていこうみたいなものを感じます。今回はごく簡単な Deployment を作ります。

Service は Deployment で定義した Pod 群がネットワーク的にどういう見え方をするかを定義します。cluster 内の Pod にはそれぞれIPアドレスが振られますが、何らかの原因で停止・復活した際には別のIPアドレスが与えられます。またオートスケールして新しい Pod が立ち上がったりもします。そういった Pod に対して正しくアクセスできなければならないので、ここでどういう名前を持つかなどを定義します。

## frontendのService作成

まず最初に frontend 側の Service を作ってみます。今回は以下のような yaml を書きました。

```yaml
kind: Service
apiVersion: v1
metadata:
  name: micro-sample-service-frontend
spec:
  type: LoadBalancer
  selector:
    app: micro-sample
    tier: frontend
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
```

大事なのは `type: LoadBalancer` です。雑にいうと、こうすると [ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) という仕組みが使われインターネットから見た時どう見えるのかといったことを定義できます。また selector でリクエストを受けて探しに行く Deployment を定義しています。

この定義を先ほど作った cluster に適用します。

```bash
kubectl apply -f frontend-service.yml
```

`kubectl get svc` とすると作られた Service が確認できます。EXTERNAL-IP のところは反映されるのに少し時間がかかります。

なお、 `kubectl describe svc micro-sample-service-frontend` などすると Service の詳細を確認できます。

## frontendのDeployment作成

次に frontend 側の Deployment を作ります。

```yaml
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: micro-sample-frontend-deployment
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: micro-sample
        tier: frontend
        track: stable
    spec:
      containers:
      - name: micro-sample-frontend
        image: gcr.io/$PROJECT_ID/$NAME:v0.1
        ports:
          - containerPort: 8080
            name: http
```

とりあえず `replicas: 2` として Pod が２つできるようにしました。image のところには先ほど Google Container Registry に push した docker image を指定します。また labels の項目は Service の selector と対応させるようにしておきます。

これを Service と同じように cluster に適用します。

```bash
kubectl apply -f frontend-deployment.yml
```

確認方法も Service のときのように

```bash
kubectl get deployment
kubectl describe deployment micro-sample-frontend-deployment
```

で行えます。

## backendのService作成

ここまでの操作で `curl "http://{EXTERNAL-IP}/"` をするときちんと200のレスポンスが返ってくるはずです（EXTERNAL-IP は `kubectl get svc` で得られるやつです）。ですがまだ backend は何もしていないため `/increment` は動きません。frontend と同じようにこちらも作っていきます。

まず Service ですが、[こちら](https://github.com/gong023/micro-sample/blob/master/infrastructure/backend-service.yml) のような定義を作りました。frontend とあまり変わりません。外部に公開しないので `type: LoadBalancer` は除いてあります。

これを適用します。

```bash
kubectl apply -f backend-service.yml
```

`kubectl get svc` すると Service が追加されているのを確認できるはずです。EXTERNAL-IP はありません。

## backendのDeployment作成

最後に backend 側の Deployment を作ります。[ここ](https://github.com/gong023/micro-sample/blob/master/infrastructure/backend-deployment.yml) にありますが、こちらもあまり frontend 側の Deployment と大差ありません。

これまでのように適用していきます。

```bash
kubectl apply -f backend-deployment.yml
```

Pod の状態は `kubectl get pods` で見れるのですが、これを実行すると先ほどの frontend 側と合わせて合計４つの Pod が立ち上がっているのが確認できると思います。

さて、ここまでで必要な設定は全て終わりです。

`curl "http://{EXTERNAL-IP}/increment?val=1"` とかするときちんと `{"val":2}` というレスポンスが得られると思います。

## クリーンアップ

最後にここまで作ったもののクリーンアップをします。今回 GKE で container cluster を作りましたが、これは Google Compute Engine という VM 上で動きます。あまり安くなかった覚えがあるので特に理由が無ければ消しておくことをお勧めします。

```bash
kubectl delete svc micro-sample-service-frontend
kubectl delete svc micro-sample-service-backend
gcloud container clusters delete micro-sample
```

## 参考

- gRPC
   - https://grpc.io/about/
- protocol buffer
   - https://developers.google.com/protocol-buffers/
- go-grpc-middleware
   - https://github.com/grpc-ecosystem/go-grpc-middleware   
- Google Container Registry
   - https://cloud.google.com/container-registry/
- kubernetes
   - https://kubernetes.io/
